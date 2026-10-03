// engine.go —— ASR 个性化识别层的快路实现（P2）。
//
// 分层（数据流单向，每层只做一件事，可单独测试）：
//
//	raw
//	 ├─ detectFillers      句界填充串清理（带守卫，见 filler.go）
//	 ├─ detectAuto         静态词表自动改写（近音触发词 / 截断还原，见 lexicon.go）
//	 ├─ resolve            区间冲突消解（排序去重叠）
//	 ├─ guard              空串守卫：删干净了 ≠ 纠干净了 → 原样返回，交上层回问
//	 └─ rebuild            按原文字节区间重建文本 + 逐条 Correction
//
//	并行支路（不改文本，只给候选，交人/交外部确认）：
//	 └─ detectCandidates   高风险专名 + 拼音近音索引（见 pinyin.go）
//
// 状态与计算分离：
//   - Correct 对当前快照是**纯函数**：只读、无副作用、可并发；
//   - 个性化状态只由 Observe 改写，快照用 atomic.Pointer 整体替换（写者串行、读者无锁）；
//   - Lexicon 导出可审计快照。
package asr

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// span 是一处**待应用**的改写，坐标是 rune 下标 [start,end)。
// 最终对外暴露的 Correction 坐标是字节下标（由 runeOffsets 换算）。
type span struct {
	start, end int
	from       string
	to         string
	kind       string
	conf       float64
	evidence   string
}

// compiled 是一次编译后的只读匹配器集合。词表变化 = 重新编译 + 整体换指针，
// 绝不在 Correct 路径里修改任何共享结构。
type compiled struct {
	trie    *trie
	py      *pinyinIndex
	entries []entry
}

// snapshot 是一个不可变的个性化状态版本。
type snapshot struct {
	compiled *compiled
	hotwords []Hotword
	version  string
}

// Personalized 是 Engine 的生产实现。
//
// 字段分两类，绝不混淆：
//   - state  : 只读快照，Correct 路径唯一访问的东西（无锁）；
//   - 其余   : 写者状态，只在 mu 内改，改完整体重编译成新快照。
type Personalized struct {
	state atomic.Pointer[snapshot]

	mu       sync.Mutex
	catal    catalog
	learned  []entry // 只由 learn 通道写回（LEARN-02+；当前恒为空）
	seen     map[string]*Hotword
	evidence []Evidence // Observe 的产物：只记证据，不做写回
	rev      int
}

// Evidence 是一次反馈的原始证据。
//
// 为什么单独存在：ASR-MODEL-02 的 L2 规定"只有 `learn` 通道可以写回持久知识"。
// 于是 `Observe` **降级为只记证据**——它记录"用户改了什么、确认与否、来源与时间"，
// 但不产生任何知识变更；真正的学习由 `learn` 通道事后消费这些证据（LEARN-02+）。
type Evidence struct {
	At        string `json:"at"`
	Raw       string `json:"raw"`
	Corrected string `json:"corrected"`
	Accepted  bool   `json:"accepted"`
	Source    string `json:"source"`
}

// evidenceCap 是内存证据条数的上限（红线 #4：不无界膨胀）。
// 超出后丢弃最旧的证据；持久化证据由 learn 通道落到 usage-events.jsonl（P2）。
const evidenceCap = 4096

// NewEngine 构造默认引擎：内置人工审定的词表 + 空的学习状态。
func NewEngine() *Personalized {
	p := &Personalized{catal: defaultCatalog(), seen: make(map[string]*Hotword)}
	p.rebuildLocked()
	return p
}

var _ Engine = (*Personalized)(nil)

// rebuildLocked 把（内置目录 + 学习所得）重编译为一个新快照并原子换入。
// 调用者必须持有 p.mu（构造期除外）。
func (p *Personalized) rebuildLocked() {
	entries := make([]entry, 0, len(p.catal.entries)+len(p.learned))
	entries = append(entries, p.catal.entries...)
	entries = append(entries, p.learned...)
	comp := compile(entries, p.catal)

	hw := make([]Hotword, 0, len(p.catal.hotwords)+len(p.seen))
	hw = append(hw, p.catal.hotwords...)
	for _, h := range p.seen {
		if h.Weight <= 0 {
			continue
		}
		hw = append(hw, *h)
	}
	sort.Slice(hw, func(i, j int) bool {
		if hw[i].Weight != hw[j].Weight {
			return hw[i].Weight > hw[j].Weight
		}
		return hw[i].Term < hw[j].Term
	})

	p.rev++
	p.state.Store(&snapshot{
		compiled: comp,
		hotwords: hw,
		version:  "vhs-asr/p2." + strconv.Itoa(p.rev),
	})
}

// Correct 纠正一条 ASR 原始文本。只读快照，无副作用，可并发。
func (p *Personalized) Correct(req CorrectRequest) CorrectResult {
	start := time.Now()
	raw := req.Raw
	res := CorrectResult{Text: raw}
	if raw == "" {
		res.Latency = time.Since(start)
		return res
	}

	comp := p.state.Load().compiled
	runes := []rune(raw)
	offs := runeOffsets(raw)

	// 无内容守卫（C3）：整句只有填充词/指代词 + 标点空白 → 不是"该纠什么"，
	// 而是"没听清/没说内容"。文本一字不动，通过 Candidates 发出**回问信号**
	// （asr.go:40：Candidates 非空 = 该问人/该问外部）。引擎绝不猜测。
	if pureNoise(runes) {
		res.Candidates = []Candidate{{
			Text:       raw, // 不提供改写建议：原样是唯一可读法
			Confidence: 0,   // 零置信 = 没有可信改写
			Reason:     askNoiseReasonPrefix + " —— 整句只有填充词/指代词，无可用内容，需上层回问澄清（引擎不猜测）",
		}}
		res.Punctuated = raw // 无内容不恢复标点
		res.Latency = time.Since(start)
		return res
	}

	applied := detectFillers(runes)
	applied = append(applied, comp.detectAuto(runes)...)
	applied = resolve(applied, len(runes))

	if len(applied) > 0 {
		text, corrs := rebuild(runes, offs, applied)
		// 空串守卫：把整句都删光不是"纠干净"，是"把人话删没了"。
		// 宁可原样返回，把判断交回上层（C3：噪声必须留给上层回问）。
		if text != raw && strings.TrimSpace(text) != "" {
			res.Text = text
			res.Corrections = corrs
		}
	}

	// 标点恢复（需求 4.3 / C1 v2）：结果只写 Punctuated + PunctuationCorrections，
	// **绝不改 Text、绝不进 Corrections**——保住 C4 的"没改 Text 不得有记录"不变式。
	res.Punctuated, res.PunctuationCorrections = punctuate(res.Text)

	// 候选支路：只建议、不改文本；非空即表示"该问人/该问外部"。
	res.Candidates = comp.detectCandidates(runes, req.Context)
	res.Latency = time.Since(start)
	return res
}

// Observe 回传一次反馈。按 ASR-MODEL-02 的 L2，**它只记证据，不写回知识**：
//   - 追加一条 Evidence（谁、何时、改了什么、是否确认、来源）；
//   - 热词权重只做频次累计，供 Lexicon 审计；
//   - **不再**登记/撤销任何词条 —— 写回持久知识的唯一通道是 `learn`（LEARN-02+）。
//
// 这样"系统变好"只有一个可审计、可关掉的入口，在线反馈不会造成不可审计的漂移。
func (p *Personalized) Observe(fb Feedback) error {
	if strings.TrimSpace(fb.Raw) == "" && strings.TrimSpace(fb.Corrected) == "" {
		return errEmptyFeedback
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.evidence = append(p.evidence, Evidence{
		At:        time.Now().UTC().Format(time.RFC3339Nano),
		Raw:       fb.Raw,
		Corrected: fb.Corrected,
		Accepted:  fb.Accepted,
		Source:    fb.Source,
	})
	if len(p.evidence) > evidenceCap {
		p.evidence = p.evidence[len(p.evidence)-evidenceCap:]
	}

	if term := strings.TrimSpace(fb.Corrected); term != "" {
		h := p.seen[term]
		if h == nil {
			h = &Hotword{Term: term, Kind: "term"}
			p.seen[term] = h
		}
		if fb.Accepted {
			h.Weight++
			h.SeenCnt++
		} else if h.Weight > 0 {
			h.Weight--
		}
	}

	p.rebuildLocked()
	return nil
}

// Evidence 导出反馈证据快照（深拷贝）。它是 `learn` 通道的输入，不是知识本身。
func (p *Personalized) Evidence() []Evidence {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Evidence, len(p.evidence))
	copy(out, p.evidence)
	return out
}

// Lexicon 导出当前个性化状态的可审计快照（深拷贝，外部改动不影响内部状态）。
func (p *Personalized) Lexicon(domain string) Lexicon {
	s := p.state.Load()
	hw := make([]Hotword, len(s.hotwords))
	copy(hw, s.hotwords)
	return Lexicon{Domain: domain, Hotwords: hw, Version: s.version}
}

// ---------------------------------------------------------------------------
// 内部工具
// ---------------------------------------------------------------------------

// runeOffsets 返回每个 rune 的起始字节下标，末尾补 len(s)，
// 于是 rune 区间 [a,b) 对应字节区间 [offs[a], offs[b])。
func runeOffsets(s string) []int {
	offs := make([]int, 0, len(s)+1)
	for i := range s {
		offs = append(offs, i)
	}
	offs = append(offs, len(s))
	return offs
}

// resolve 过滤非法/空操作 span，按位置排序并去掉重叠（保留先出现且更长者）。
func resolve(spans []span, n int) []span {
	out := make([]span, 0, len(spans))
	for _, s := range spans {
		if s.start < 0 || s.end > n || s.start >= s.end || s.from == s.to {
			continue
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].start != out[j].start {
			return out[i].start < out[j].start
		}
		return out[i].end > out[j].end
	})
	var res []span
	for _, s := range out {
		if n := len(res); n > 0 && s.start < res[n-1].end {
			continue
		}
		res = append(res, s)
	}
	return res
}

// rebuild 按 span 重建文本，并产出字节区间可回溯的 Correction。
func rebuild(runes []rune, offs []int, spans []span) (string, []Correction) {
	var b strings.Builder
	var corrs []Correction
	prev := 0
	for _, s := range spans {
		if s.start < prev || s.end > len(runes) {
			continue
		}
		b.WriteString(string(runes[prev:s.start]))
		b.WriteString(s.to)
		corrs = append(corrs, Correction{
			Start:      offs[s.start],
			End:        offs[s.end],
			From:       string(runes[s.start:s.end]),
			To:         s.to,
			Kind:       s.kind,
			Confidence: s.conf,
			Evidence:   s.evidence,
		})
		prev = s.end
	}
	b.WriteString(string(runes[prev:]))
	return b.String(), corrs
}

// 以下三个 helper 供 **learn 通道的写回路径**使用（LEARN-02+，等模型与协议确定）。
// 按 L2 只有 learn 会调用它们；`Observe` 已不再调用（它只记证据），
// 因此目前没有调用点——这是刻意的，不是死代码。

// upsertLearned 登记/强化一条学习词条（同 from→to 只留一条并加权）。
func upsertLearned(list []entry, e entry) []entry {
	for i := range list {
		if list[i].from == e.from && list[i].to == e.to {
			list[i].conf = minFloat(0.99, list[i].conf+0.05)
			return list
		}
	}
	return append(list, e)
}

func removeLearned(list []entry, from, to string) []entry {
	out := list[:0]
	for _, e := range list {
		if e.from == from && e.to == to {
			continue
		}
		out = append(out, e)
	}
	return out
}

func sourceConf(src string) float64 {
	switch src {
	case "user_edit":
		return 0.90
	case "reviewer":
		return 0.95
	case "implicit":
		return 0.60
	default:
		return 0.70
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
