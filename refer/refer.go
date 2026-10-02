// Package refer 是指代消解层（设计 v2 §14）：把口语里的模糊表述（"它/那个文件/上次"）
// 逐层消解为具体实体：词典层(100%) → 上下文规则层 → 语言层(可选) → 低置信回问。
// 规则层低置信绝不静默吃掉：要么交 ModelFn，要么显式 Ask；跨域歧义必须回问"哪个域？"。
// 本包只依赖标准库 + contract + memory。
package refer

// 【伪代码逻辑层】（评审关卡产物；分层消解权威定义在设计 v2 §14.1，
//  本层只描述单模块控制流/分支/拒绝路径/异常处理，规则语义标注"搬 VSL"。）
//
// Resolve(intent, spaceID) -> *Intent：
//   0. 若 intent.Target.Entity 已显式（explicit）：仅走词典层规范化，直接返回（不回问）。
//   1. 词典层（搬 VSL：实体词典 100%）：
//      在 CorrectedText 中命中词典 Term/Variant → Target.Entity=Term, RefType="dict"，返回。
//   2. 指代触发检测（搬 VSL：它/那个文件/这个/上次/之前那个）：
//      若无指代词 → 无需消解，原样返回。
//   3. 上下文规则层（搬 VSL：作用域内最近实体）：
//      cands = r.Recent 按 kind 过滤（"那个文件"→file；"它"→优先 file，兜底任意）
//      排序键：(space==spaceID 优先, Ts 倒序=新者优先)
//      if cands 空 → 跳 4（语言层/Ask）
//      if 最优唯一（spaceID 命中 或 新 Ts 明显领先）→ 取之为 Target, RefType="anaphora"，返回
//      if cands 跨多个 space 且无 spaceID 命中（跨域歧义）→ Ask「你说的是哪个域？」
//      if 顶级平局（同 space 同 Ts / 多候选并列）→ Ask，不猜
//   4. 语言层（可选，搬 VSL：候选+置信度）：
//      if r.ModelFn != nil:
//         entity, conf = r.ModelFn(CorrectedText, 候选实体名)
//         if conf >= 阈值: Target=entity, RefType="model"，返回
//      // 规则层低置信不许静默吃掉：落到这里必须 Ask
//      Ask「你说的「<代词>」指的是哪个？」
//   异常：Dict 为 nil → 词典层跳过（不报错）；Recent 为空 → 直接走语言层/Ask。
//
// Solidify(entity, variant) -> error：
//   确认后的别名固化进个人词典（memory.Dictionary.AddTerm，source=model）。
//
// ResolveOptions(intent, spaceID) -> (*Intent, []Option, error)【M4 选项按钮化】：
//   // 与 Resolve 同一套分层；歧义回问时额外产出结构化候选（2-4 个，供手机端点选续跑）。
//   候选来源优先级（搬 VSL：模糊→精确）：
//     1) 词典命中项：文本命中的词典 Term → Option{ID:"dict:"+term, Label:term+"（词典）"}
//     2) 上下文最近实体：r.Recent 按 (spaceID 优先, Ts 新) 排序后取前 N
//        → Option{ID:"rec:"+entity, Label:kind 前缀+实体+(跨域时标注 域:xxx)}
//   数量上限 4；按 ID 去重；词典候选排前、最近实体随后。
//   仅在【歧义回问分支】产出候选：跨域歧义 / 顶级平局。
//   目标不存在或无候选（Recent 空 + 词典无命中）→ Options=[]，仅 Ask 文本（不伪造选项）。
//   Resolve 保留冻结签名，内部转调本函数并丢弃 Options。

import (
	"sort"
	"strings"

	"voicesign-harness/contract"
	"voicesign-harness/memory"
)

// RecentEntity 是 pipeline 注入的最近实体（按时间追加；指代消解的上下文来源）。
type RecentEntity struct {
	Space  string // 所属域
	Entity string // 具体实体（文件名/项目名/人名）
	Kind   string // file | project | person | space
	Ts     string // ISO 时间戳（字符串倒序即"更新"，要求调用方给可字典序比较的格式）
}

// Resolver 是指代消解器：词典层 + 上下文规则层 + 可选语言层。
type Resolver struct {
	Dict    *memory.Dictionary
	Recent  []RecentEntity
	ModelFn func(q string, cands []string) (entity string, conf float64)
}

// Option 是一个可点选的候选目标（M4 选项按钮化；D 消费为 AskOption{ID,Label}）。
// ID 稳定可测（"dict:"+term / "rec:"+entity）；Label 中文可读。
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// New 构造消解器（dict 可为 nil，词典层自动跳过）。
func New(dict *memory.Dictionary) *Resolver {
	return &Resolver{Dict: dict}
}

// anaphoraTriggers 是中文口语指代词（长词优先，CJK 无词边界，子串匹配）。
var anaphoraTriggers = []string{
	"那个文件", "那个客户", "那个项目", "上次那个", "之前那个",
	"上次", "之前", "那个", "这个文件", "这个", "它", "他",
}

// fileTriggers 表示指代指向"文件"类实体。
var fileTriggers = []string{"那个文件", "这个文件", "它"}

// modelConfThreshold 是语言层放行阈值。
const modelConfThreshold = 0.6

func hasAny(text string, words []string) (string, bool) {
	for _, w := range words {
		if strings.Contains(text, w) {
			return w, true
		}
	}
	return "", false
}

// dictLookup 在文本中查词典命中（返回规范词；未命中返回空）。
func (r *Resolver) dictLookup(text string) string {
	if r.Dict == nil {
		return ""
	}
	low := strings.ToLower(text)
	for _, t := range r.Dict.Terms {
		if strings.Contains(low, strings.ToLower(t.Term)) {
			return t.Term
		}
		for _, v := range t.Variants {
			if v != "" && strings.Contains(low, strings.ToLower(v)) {
				return t.Term
			}
		}
	}
	return ""
}

// rankCands 按 (spaceID 优先, Ts 倒序) 排序候选，返回排好序的切片。
func rankCands(cands []RecentEntity, spaceID string) []RecentEntity {
	out := make([]RecentEntity, len(cands))
	copy(out, cands)
	sort.SliceStable(out, func(i, j int) bool {
		si := out[i].Space == spaceID
		sj := out[j].Space == spaceID
		if si != sj {
			return si // space 命中者优先
		}
		return out[i].Ts > out[j].Ts // Ts 倒序（新者优先；ISO 字符串字典序=时间序）
	})
	return out
}

// Resolve 执行分层消解，回填 intent.Target；歧义时写 intent.Ask（不猜执行）。
// 保留 M2 冻结签名；结构化候选见 ResolveOptions。
func (r *Resolver) Resolve(it *contract.Intent, spaceID string) (*contract.Intent, error) {
	got, _, err := r.ResolveOptions(it, spaceID)
	return got, err
}

// ResolveOptions 与 Resolve 同一套分层；歧义回问时额外返回 2-4 个结构化候选目标（M4）。
func (r *Resolver) ResolveOptions(it *contract.Intent, spaceID string) (*contract.Intent, []Option, error) {
	if it == nil {
		return nil, nil, nil
	}
	var opts []Option

	// 0. 已显式目标：仅词典规范化，不回问
	if it.Target != nil && it.Target.Entity != "" {
		if canon := r.dictLookup(it.CorrectedText); canon != "" {
			it.Target.Entity = canon
		}
		return it, opts, nil
	}

	text := it.CorrectedText

	// 缺口 G4：UNKNOWN 的澄清原因**不得**被指代回问覆写。
	//
	// 分类器判 UNKNOWN 时会写「你是想让我做什么？」——这是用户最需要知道的信息。
	// 若 refer 把它改写成「你说的「那个」指的是哪个？」，用户会被问一个**错误的问题**。
	//
	// 但不能一刀切（既有回归 pipeline.TestCodexNineRegressions#1 要求
	// 「我现在想认真开始测…把这个哈…推进起来…」这一句**必须**保留 refer 的指代回问）。
	// 区分标准：是不是**操作指代**。
	//   - 「把 这个…」→ 操作指代，refer 的澄清有价值 → 放行；
	//   - 「嗯 那个 呃 记一下」→ 语气词，不是操作对象 → 保留分类器的澄清原因。
	if it.Intent == contract.IntentUnknown && it.Ask != "" && !anyOperationAnaphora(text) {
		return it, opts, nil
	}

	// 1. 词典层（100%）
	if canon := r.dictLookup(text); canon != "" {
		it.Target = &contract.Target{Entity: canon, RefType: "dict"}
		return it, opts, nil
	}

	// 2. 指代触发检测
	trigger, ok := hasAny(text, anaphoraTriggers)
	if !ok {
		return it, opts, nil // 无指代，无需消解
	}

	// 3. 上下文规则层：按 kind 过滤
	wantKind := ""
	if _, isFile := hasAny(text, fileTriggers); isFile {
		wantKind = "file"
	}
	var pool []RecentEntity
	for _, e := range r.Recent {
		if wantKind == "" || e.Kind == wantKind {
			pool = append(pool, e)
		}
	}
	if len(pool) == 0 && wantKind != "" { // 文件类无命中 → 兜底任意
		for _, e := range r.Recent {
			pool = append(pool, e)
		}
	}

	if len(pool) > 0 {
		ranked := rankCands(pool, spaceID)
		top := ranked[0]
		// 跨域歧义：没有任何候选命中 spaceID，且候选分布在 >1 个域
		spaces := map[string]bool{}
		for _, e := range ranked {
			spaces[e.Space] = true
		}
		matchedSpace := top.Space == spaceID
		// 顶级平局：次优与最优同 space 同 Ts
		tie := len(ranked) > 1 &&
			ranked[1].Space == top.Space && ranked[1].Ts == top.Ts
		switch {
		case matchedSpace:
			it.Target = &contract.Target{Entity: top.Entity, RefType: "anaphora"}
			return it, opts, nil
		case !matchedSpace && len(spaces) > 1:
			it.Ask = "你说的「" + trigger + "」是指哪个域？我看到多个域里都有最近操作"
			opts = r.buildOptions(text, ranked)
			return it, opts, nil
		case tie:
			it.Ask = "你说的「" + trigger + "」具体指哪一个？有多个相近的对象"
			opts = r.buildOptions(text, ranked)
			return it, opts, nil
		default:
			// 单候选（虽跨域但唯一）→ 直接消解
			it.Target = &contract.Target{Entity: top.Entity, RefType: "anaphora"}
			return it, opts, nil
		}
	}

	// 4. 语言层（可选）
	if r.ModelFn != nil {
		candNames := make([]string, 0, len(r.Recent))
		for _, e := range r.Recent {
			candNames = append(candNames, e.Entity)
		}
		if entity, conf := r.ModelFn(text, candNames); conf >= modelConfThreshold && entity != "" {
			it.Target = &contract.Target{Entity: entity, RefType: "model"}
			return it, opts, nil
		}
	}

	// 规则层低置信不许静默吃掉 → 显式回问（无候选则 Options 为空）
	it.Ask = "你说的「" + trigger + "」指的是哪个？请再说清楚一点"
	return it, opts, nil
}

// kindPrefix 按实体类型给中文可读前缀。
func kindPrefix(kind string) string {
	switch kind {
	case "file":
		return "那个文件："
	case "project":
		return "项目："
	case "person":
		return "人："
	case "space":
		return "域："
	default:
		return ""
	}
}

// buildOptions 从词典命中 + 排序后的最近实体构造 2-4 个结构化候选（去重、上限 4）。
func (r *Resolver) buildOptions(text string, ranked []RecentEntity) []Option {
	seen := map[string]bool{}
	var opts []Option
	// 1) 词典命中项排前
	if r.Dict != nil {
		low := strings.ToLower(text)
		for _, t := range r.Dict.Terms {
			if !strings.Contains(low, strings.ToLower(t.Term)) {
				continue
			}
			id := "dict:" + t.Term
			if !seen[id] {
				seen[id] = true
				opts = append(opts, Option{ID: id, Label: t.Term + "（词典）"})
			}
		}
	}
	// 2) 上下文最近实体
	for _, e := range ranked {
		id := "rec:" + e.Entity
		if seen[id] {
			continue
		}
		seen[id] = true
		label := kindPrefix(e.Kind) + e.Entity
		if e.Space != "" {
			label += "（域:" + e.Space + "）"
		}
		opts = append(opts, Option{ID: id, Label: label})
	}
	if len(opts) > 4 {
		opts = opts[:4]
	}
	return opts
}

// Solidify 把确认后的别名固化进个人词典（AddTerm，source=voice）。
func (r *Resolver) Solidify(entity, variant string) error {
	if r.Dict == nil {
		return nil
	}
	term := memory.Term{Term: entity, Category: "voice", Source: "voice"}
	if variant != "" && variant != entity {
		term.Variants = []string{variant}
	}
	if err := r.Dict.AddTerm(term); err != nil {
		return err
	}
	return nil
}

// operationVerbs 是指代词后紧跟时说明它是"操作对象"的动词。
var operationVerbs = []rune("发删改查看开关跑修记提部打建写读")

// operationAnaphora 报告某个指代词是否构成"操作指代"。
//
//	把 这个 改一下     → 前一字是"把"          → 是操作指代
//	那个文件 改一下     → 后一字是动词"改"       → 是操作指代
//	嗯 那个 呃 记一下   → 前后都不是操作语境      → 只是语气词，不是操作对象
func operationAnaphora(text, trigger string) bool {
	i := strings.Index(text, trigger)
	if i < 0 {
		return false
	}
	if i > 0 {
		prev := []rune(text[:i])
		if len(prev) > 0 {
			p := prev[len(prev)-1]
			switch p {
			case '把', '将', '对', '给':
				return true
			}
			// 动词在指代词之前：「打开它」「删除这个」——它仍是操作对象。
			for _, v := range operationVerbs {
				if p == v {
					return true
				}
			}
		}
	}
	rest := []rune(text[i+len(trigger):])
	if len(rest) > 0 {
		for _, v := range operationVerbs {
			if rest[0] == v {
				return true
			}
		}
	}
	return false
}

// anyOperationAnaphora 报告文本中是否存在**任意**一个操作指代。
//
// 注意必须遍历全部候选：hasAny 按词表顺序返回首个命中，而词表顺序与出现位置无关，
// 长句里可能先命中语气词"那个"，却漏掉更早出现的操作指代"把这个"。
func anyOperationAnaphora(text string) bool {
	for _, t := range anaphoraTriggers {
		if strings.Contains(text, t) && operationAnaphora(text, t) {
			return true
		}
	}
	return false
}
