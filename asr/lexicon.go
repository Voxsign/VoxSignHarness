// lexicon.go —— 词表目录（人工审定）+ 匹配 + 在线学习的状态容器。
//
// 词条分两类，**安全等级不同**：
//
//	auto = true   **自动改写**。只允许人工审定、有明确语料依据、且不涉及专名的
//	              低风险条目（多字触发词近音、截断还原）。当前只有两条。
//	auto = false  **只出候选**（Candidate）。高风险条目一律走这条路：
//	              专名/英文/域名被 ASR 撕碎（deept→DeepSeek、哈尼斯→harness…），
//	              以及拼音近音索引给出的建议。原因：改错比不改糟（任务书 §5.2）。
//
// 为什么专名只给候选而不自动纠：
//   - 语料里同一专名一次发言内有多种错法（obs-03 deept / deep sick），
//     选定一个唯一答案的证据不足；
//   - C1 保真要求这些 **observed** 长句一个字不得动。
//
// 学习策略同样保守：Observe 学到的映射只登记为候选词条，绝不自动改写。
package asr

import (
	"errors"
	"sort"
	"strings"
)

var errEmptyFeedback = errors.New("asr: feedback 既无 raw 也无 corrected，无法学习")

// maxCandidates 是一次 Correct 最多给出的候选数，避免把噪音全倒给上层。
const maxCandidates = 3

// entry 是一条词表条目。
type entry struct {
	from, to string
	kind     string
	conf     float64
	evidence string
	auto     bool // true = 自动改写；false = 只出候选

	// 以下由 compile 依 from 推导，用于拉丁词边界判定。
	latinStart, latinEnd bool
}

// catalog 是内置目录：词条 + 拼音热词 + 音节表 + 审定热词。
type catalog struct {
	entries  []entry
	terms    []pinyinTerm
	table    map[rune]string
	hotwords []Hotword
}

// defaultCatalog 返回内置目录（人工审定，非学习所得）。
func defaultCatalog() catalog {
	return catalog{
		entries:  defaultEntries(),
		terms:    defaultPinyinTerms(),
		table:    buildPinyinTable(),
		hotwords: defaultHotwords(),
	}
}

// defaultEntries 是人工审定的词表。每条都要能回答"语料里哪一句支撑它"。
func defaultEntries() []entry {
	return []entry{
		// ---- 自动改写（低风险、有语料依据）----------------------------------
		{
			from: "题交", to: "提交", kind: "homophone", auto: true, conf: 0.92,
			evidence: "近音词表命中：题交(tí jiāo) → 提交(tí jiāo)；多字触发词，还原后整条链路才通（real-02）",
		},
		{
			from: "别字", to: "错别字", kind: "truncation", auto: true, conf: 0.80,
			evidence: "截断还原：别字 → 错别字（ASR 漏掉前字「错」）（real-03）；守卫见 truncationGuard",
		},

		// ---- 只出候选（高风险专名/英文，绝不自动改写）-----------------------
		{
			from: "deept", to: "DeepSeek", kind: "hotword", conf: 0.62,
			evidence: "专名候选：deept ≈ DeepSeek（obs-03；同一专名的另一种错法见 deep sick）",
		},
		{
			from: "deep sick", to: "DeepSeek", kind: "hotword", conf: 0.70,
			evidence: "专名候选：deep sick ≈ DeepSeek（obs-03/obs-18，同一错法重复出现=个性化词表可用信号）",
		},
		{
			from: "哈尼斯", to: "harness", kind: "hotword", conf: 0.60,
			evidence: "专名候选：哈尼斯 ≈ harness（obs-10，同一句内两种错法之一）",
		},
		{
			from: "汉尼斯", to: "harness", kind: "hotword", conf: 0.60,
			evidence: "专名候选：汉尼斯 ≈ harness（obs-10，同一句内两种错法之二）",
		},
		{
			from: "issanghannes", to: "harness", kind: "hotword", conf: 0.45,
			evidence: "专名候选：issanghannes ≈ harness（obs-19，撕碎到几乎不可复原，必须人工确认）",
		},
		{
			from: "voice sound harnessnes", to: "voice-sign harness", kind: "hotword", conf: 0.55,
			evidence: "专名候选：voice sound harnessnes ≈ voice-sign harness（obs-01）",
		},
		{
			from: "P图做点com", to: "peterzou.com", kind: "hotword", conf: 0.66,
			evidence: "专名候选：P图做点com ≈ peterzou.com（obs-17，域名被彻底撕碎）",
		},
		{
			from: "AR", to: "ASR", kind: "hotword", conf: 0.40,
			evidence: "专名候选：AR ≈ ASR（obs-10/obs-18）；仅 2 字母，易与普通缩写混淆，务必人工确认",
		},
	}
}

// defaultPinyinTerms 是参与拼音近音索引的正确写法。
// 这些条目**只产出候选**——拼音匹配能召回，但不足以直接改写。
func defaultPinyinTerms() []pinyinTerm {
	return []pinyinTerm{
		{canonical: "提交", syllables: []string{"ti", "jiao"}, conf: 0.55, note: "多字触发词"},
		{canonical: "通过", syllables: []string{"tong", "guo"}, conf: 0.50, note: "缺字候选（obs-14 登记类别）"},
		{canonical: "报价单", syllables: []string{"bao", "jia", "dan"}, conf: 0.55, note: "槽位专名"},
		{canonical: "库存", syllables: []string{"ku", "cun"}, conf: 0.50, note: "槽位专名"},
		{canonical: "测试", syllables: []string{"ce", "shi"}, conf: 0.45, note: "常用词"},
	}
}

// defaultHotwords 是内置热词（人工审定，不是学习所得；SeenCnt 无统计意义，固定 1）。
func defaultHotwords() []Hotword {
	return []Hotword{
		{Term: "voice-sign harness", Kind: "project", Weight: 1.0, SeenCnt: 1},
		{Term: "DeepSeek", Kind: "project", Weight: 1.0, SeenCnt: 1},
		{Term: "peterzou.com", Kind: "project", Weight: 1.0, SeenCnt: 1},
		{Term: "ASR", Kind: "term", Weight: 1.0, SeenCnt: 1},
		{Term: "提交", Kind: "command", Weight: 1.0, SeenCnt: 1},
		{Term: "错别字", Kind: "term", Weight: 1.0, SeenCnt: 1},
	}
}

// compile 把词条编译成只读匹配器集合。
func compile(entries []entry, c catalog) *compiled {
	es := make([]entry, len(entries))
	for i, e := range entries {
		if e.from != "" {
			rs := []rune(e.from)
			e.latinStart = isASCIIAlnum(rs[0])
			e.latinEnd = isASCIIAlnum(rs[len(rs)-1])
		}
		es[i] = e
	}
	return &compiled{
		trie:    newTrie(es),
		py:      newPinyinIndex(c.terms, c.table),
		entries: es,
	}
}

// detectAuto 找出可**自动改写**的区间。每个位置取最长且通过守卫的 auto 条目。
func (c *compiled) detectAuto(runes []rune) []span {
	var out []span
	for i := 0; i < len(runes); i++ {
		best, bestLen := -1, -1
		for _, k := range c.trie.matchAt(runes, i) {
			e := c.entries[k]
			if !e.auto {
				continue
			}
			n := len([]rune(e.from))
			end := i + n
			if e.latinStart && i > 0 && isASCIIAlnum(runes[i-1]) {
				continue
			}
			if e.latinEnd && end < len(runes) && isASCIIAlnum(runes[end]) {
				continue
			}
			if e.kind == "truncation" && !truncationGuard(runes, i, e) {
				continue
			}
			if n > bestLen {
				bestLen, best = n, k
			}
		}
		if best < 0 {
			continue
		}
		e := c.entries[best]
		out = append(out, span{
			start: i, end: i + bestLen, from: string(runes[i : i+bestLen]), to: e.to,
			kind: e.kind, conf: e.conf, evidence: e.evidence,
		})
		i += bestLen - 1
	}
	return out
}

// truncationGuard 是截断还原的守卫：不把已经是完整正确词的片段再套一层
// （否则「错别字」→「错错别字」）。
func truncationGuard(runes []rune, i int, e entry) bool {
	if i > 0 && runes[i-1] == '错' {
		return false
	}
	target := []rune(e.to)
	if i+len(target) <= len(runes) && string(runes[i:i+len(target)]) == e.to {
		return false
	}
	return true
}

// detectCandidates 产出**不改文本**的候选：高风险专名 + 拼音近音索引。
//
// 候选非空 = 本层明确表示"该问人/该问外部"，这是 C4 可观测的另一半。
func (c *compiled) detectCandidates(runes []rune, ctx []string) []Candidate {
	best := make(map[string]Candidate)
	add := func(text, reason string, conf float64) {
		if text == "" {
			return
		}
		if cur, ok := best[text]; ok && cur.Confidence >= conf {
			return
		}
		best[text] = Candidate{Text: text, Confidence: conf, Reason: reason}
	}

	// 1) 静态候选词表（专名/英文/域名）。
	for i := 0; i < len(runes); i++ {
		for _, k := range c.trie.matchAt(runes, i) {
			e := c.entries[k]
			if e.auto {
				continue
			}
			n := len([]rune(e.from))
			end := i + n
			if e.latinStart && i > 0 && isASCIIAlnum(runes[i-1]) {
				continue
			}
			if e.latinEnd && end < len(runes) && isASCIIAlnum(runes[end]) {
				continue
			}
			add(e.to, e.evidence, e.conf)
		}
	}

	// 2) 拼音近音索引：定长滑窗，窗口全字可查且音节命中才产出候选。
	if c.py != nil {
		for i := 0; i < len(runes); i++ {
			for _, l := range c.py.lens {
				if i+l > len(runes) {
					break
				}
				key, ok := c.py.keyOf(runes[i : i+l])
				if !ok {
					continue
				}
				w := string(runes[i : i+l])
				for _, t := range c.py.byKey[key] {
					if w == t.canonical {
						continue // 已经是正确写法，不该出候选
					}
					reason := "拼音近音候选：" + w + "（" + strings.Join(t.syllables, " ") + "）≈ " + t.canonical
					if t.note != "" {
						reason += "；" + t.note
					}
					add(t.canonical, reason, t.conf)
				}
			}
		}
	}

	out := make([]Candidate, 0, len(best))
	for _, cand := range best {
		if contextHas(ctx, cand.Text) {
			cand.Confidence = minFloat(0.95, cand.Confidence+0.10)
			cand.Reason += "；近期上下文出现过，置信度上调"
		}
		out = append(out, cand)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Text < out[j].Text
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}

// contextHas 报告近期上下文里是否出现过某词（用于候选排序，不改变文本）。
func contextHas(ctx []string, term string) bool {
	for _, s := range ctx {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}
