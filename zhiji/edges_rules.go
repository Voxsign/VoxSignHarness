// edges_rules.go —— 知己 · semantic / causal 规则版自动落边（P2 任务）。
//
// 纯代码判断、不调任何外部模型/网络；边一律走 Graph.Add（自动去重加权，同 from,to,rel
// 取大权重）。temporal / entity 两类边已在别处落地，本文件只新增 semantic 与 causal：
//
//	semantic：两段文本「关键词∪实体」共享 >=2 → 无向边（Weight=0.6）
//	causal  ：同一文本命中因果连接词 → 有向边（Weight=0.7，方向=原因→结果）
//
// P0 全是粗规则（无分词器/无 NER），P2 换模型做子句级归因与实体抽取。
package zhiji

import "strings"

// causalWords 因果连接词表（P0 冻结；与 systemone.go Route 的因果词是超集）。
var causalWords = []string{"因为", "所以", "导致", "使得", "因此", "于是", "从而", "鉴于", "造成"}

// effectLeadWords 结果侧引导词：命中即把文本从该处切成 [原因侧 | 结果侧]。
var effectLeadWords = []string{"所以", "因此", "于是", "从而", "导致", "使得", "造成"}

// cnStopRunes 中文停用字：抽词时按 rune 粗过滤（无分词器的 P0 占位启发式）。
var cnStopRunes = map[rune]bool{
	'我': true, '你': true, '他': true, '她': true, '它': true, '们': true,
	'的': true, '了': true, '是': true, '在': true, '和': true, '与': true,
	'也': true, '就': true, '还': true, '把': true, '被': true, '让': true,
	'向': true, '从': true, '到': true, '要': true, '会': true, '能': true,
	'个': true, '这': true, '那': true, '有': true, '不': true, '没': true,
	'很': true, '又': true, '或': true, '及': true, '之': true, '于': true,
	'而': true, '但': true, '其': true, '此': true, '该': true,
}

// enStopWords 英文停用词（小写形式）。
var enStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "and": true, "or": true, "of": true,
	"to": true, "in": true, "on": true, "for": true, "with": true,
	"at": true, "by": true, "from": true, "as": true, "it": true,
	"its": true, "this": true, "that": true, "be": true, "been": true,
}

// isCJK 是否 CJK 统一表意文字（P0 粗判，不含扩展区）。
func isCJK(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }

// extractKeywords 从一段文本抽「关键词集合」（去重，顺序不定）：
//
//	英文：连续字母数字串小写化，长度>=2 且不在英文停用词表；
//	中文：按标点/英文/数字切段，段内去中文停用字，再对剩余连续汉字滑二元组（bigram）。
//
// 例：「STC 套餐办理」→ {stc, 套餐, 餐办, 办理}（bigram 为 P0 占位，P2 换分词）。
func extractKeywords(text string) []string {
	seen := map[string]struct{}{}
	add := func(w string) {
		if w != "" {
			seen[w] = struct{}{}
		}
	}
	var ascii strings.Builder
	flushASCII := func() {
		if ascii.Len() == 0 {
			return
		}
		w := strings.ToLower(ascii.String())
		ascii.Reset()
		if len(w) >= 2 && !enStopWords[w] {
			add(w)
		}
	}
	var cjk strings.Builder
	flushCJK := func() {
		if cjk.Len() == 0 {
			return
		}
		rs := []rune(cjk.String())
		cjk.Reset()
		var clean []rune
		for _, r := range rs {
			if !cnStopRunes[r] {
				clean = append(clean, r)
			}
		}
		for i := 0; i+1 < len(clean); i++ {
			add(string(clean[i : i+2]))
		}
	}
	for _, r := range text {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			flushCJK()
			ascii.WriteRune(r)
		case isCJK(r):
			flushASCII()
			cjk.WriteRune(r)
		default: // 标点/空白/符号：两种缓冲都切一刀
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	return out
}

// extractEntities 从一段文本抽「实体集合」（P0 粗抽，去重）：
//
//	大写英文专名：连续 >=2 个大写字母，如 STC / CPE / DNA；
//	中文专名片段：按非汉字切段，段内去停用字后长度>=2 的连续汉字串（整段一个实体）。
//
// P0 占位启发式；P2 换 NER 模型。
func extractEntities(text string) []string {
	seen := map[string]struct{}{}
	var upper strings.Builder
	flushUpper := func() {
		if upper.Len() == 0 {
			return
		}
		w := upper.String()
		upper.Reset()
		if len(w) >= 2 {
			seen[w] = struct{}{}
		}
	}
	for _, r := range text {
		if r >= 'A' && r <= 'Z' {
			upper.WriteRune(r)
			continue
		}
		flushUpper()
	}
	flushUpper()
	for _, seg := range strings.FieldsFunc(text, func(r rune) bool { return !isCJK(r) }) {
		rs := []rune(seg)
		clean := make([]rune, 0, len(rs))
		for _, r := range rs {
			if !cnStopRunes[r] {
				clean = append(clean, r)
			}
		}
		if len(clean) >= 2 {
			seen[string(clean)] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	return out
}

// sharedUnionSize 计算两段文本「关键词∪实体」小写化后的去重共享元素个数。
// 英文专名（STC）在关键词侧小写化为 stc、实体侧小写化也为 stc，并集自动合并不重复计数。
func sharedUnionSize(textA, textB string) int {
	setA := map[string]struct{}{}
	for _, w := range extractKeywords(textA) {
		setA[strings.ToLower(w)] = struct{}{}
	}
	for _, w := range extractEntities(textA) {
		setA[strings.ToLower(w)] = struct{}{}
	}
	n := 0
	touch := func(w string) {
		k := strings.ToLower(w)
		if _, ok := setA[k]; ok {
			n++
			delete(setA, k) // 去重：同一元素只计一次
		}
	}
	for _, w := range extractKeywords(textB) {
		touch(w)
	}
	for _, w := range extractEntities(textB) {
		touch(w)
	}
	return n
}

// MaybeAddSemantic 若 A、B 两段文本「关键词+实体」并集共享 >=2 个元素，
// 落一条无向 semantic 边（Weight=0.6）；否则图不动。空文本/同节点安全不落边。
func MaybeAddSemantic(g *Graph, idA, idB string, textA, textB string) {
	if g == nil || idA == "" || idB == "" || idA == idB {
		return
	}
	if sharedUnionSize(textA, textB) >= 2 {
		g.Add(Edge{From: idA, To: idB, Rel: EdgeRelSemantic, Directed: false, Weight: 0.6})
	}
}

// MaybeAddCausal 若文本命中任一因果连接词，即认为 fromNode（原因侧）导致 toNode（结果侧），
// 落一条有向 causal 边（Weight=0.7）。方向由调用方按子句先后给出；纯并列/无连接词文本不落边。
func MaybeAddCausal(g *Graph, fromNode, toNode string, text string) {
	if g == nil || fromNode == "" || toNode == "" || fromNode == toNode {
		return
	}
	if !hasAny(text, causalWords) { // 复用 systemone.go 的大小写不敏感包含判断
		return
	}
	g.Add(Edge{From: fromNode, To: toNode, Rel: EdgeRelCausal, Directed: true, Weight: 0.7})
}

// splitCausalClauses 用因果连接词把一段文本粗切成 [原因侧 | 结果侧] 两半。
//
//	P0 规则：优先在结果侧引导词（所以/因此/于是/从而/导致/使得/造成）处切开——
//	         连接词之前是原因侧、之后是结果侧；两半都非空才 ok=true。
//	         「因为/鉴于」只标原因侧起点，P0 不做无结果侧的半切（ok=false）。
//	注释：P0 是按连接词位置的粗切，不做子句依存分析；P2 换模型做精确因果归因。
func splitCausalClauses(text string) (cause, effect string, ok bool) {
	for _, conn := range effectLeadWords {
		if i := strings.Index(text, conn); i >= 0 { // i 为字节偏移，conn 同 UTF-8 字节长
			cause = strings.TrimSpace(text[:i])
			effect = strings.TrimSpace(text[i+len(conn):])
			return cause, effect, cause != "" && effect != ""
		}
	}
	return "", "", false
}
