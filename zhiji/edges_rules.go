// edges_rules.go —— 知己 · semantic / causal 规则版自动落边（P2 任务）。
//
// 纯代码判断、不调任何外部模型/网络；边一律走 Graph.Add（自动去重加权，同 from,to,rel
// 取大权重）。temporal / entity 两类边已在别处落地，本文件只新增 semantic 与 causal：
//
//	semantic：两段文本「关键词∪实体」共享 >=2 → 无向边（Weight=0.6）
//	causal  ：同一文本命中因果连接词且能切出原因/结果两侧 → 有向边（Weight=0.7）
//
// P0 全是粗规则（无分词器/无 NER），P2 换模型做子句级归因与实体抽取。
package zhiji

import "strings"

// causalWords 因果连接词表（P0 冻结；与 systemone.go Route 的因果词是超集）。
var causalWords = []string{"因为", "所以", "导致", "使得", "因此", "于是", "从而", "鉴于", "造成"}

// effectLeadWords 结果侧引导词：命中即把该子句从该处切成 [原因侧 | 结果侧]。
var effectLeadWords = []string{"所以", "因此", "于是", "从而", "导致", "使得", "造成"}

// causeLeadWords 原因侧引导词：命中则其后续子句视为结果侧。
var causeLeadWords = []string{"因为", "鉴于"}

// clauseSplitterChars 中文/英文标点：按它们把整段文本切成子句。
const clauseSplitterChars = "，。！？；、,."

// cnStopRunes 中文停用字：抽词时一旦出现即视为边界（且本身绝不入词/入实体）。
// 注意：是「切分边界」不是「删除后桥接」——否则「昨天的咖啡」删字后会拼出噪声 bigram「天咖」。
var cnStopRunes = map[rune]bool{
	'我': true, '你': true, '他': true, '她': true, '它': true, '们': true,
	'的': true, '了': true, '是': true, '在': true, '和': true, '与': true,
	'也': true, '就': true, '都': true, '还': true, '把': true, '被': true,
	'让': true, '向': true, '从': true, '到': true, '要': true, '会': true,
	'能': true, '个': true, '这': true, '那': true, '有': true, '不': true,
	'没': true, '很': true, '又': true, '或': true, '及': true, '之': true,
	'于': true, '而': true, '但': true, '其': true, '此': true, '该': true,
	'上': true, '下': true, '着': true, '过': true, '吗': true, '呢': true,
	'吧': true, '啊': true, '说': true, '去': true, '来': true, '做': true,
}

// enStopWords 英文停用词（小写形式匹配；数字串与大写英文专名不受此表影响）。
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
//	英文：连续字母数字串小写化，长度>=2 且不在英文停用词表（数字串 230sar/5g 照常保留）；
//	中文：按标点/英文/数字/中文停用字切句，段内对连续汉字滑二元组（bigram）。
//	      含停用字的 bigram 天然不产生（停用字即切分点），纯停用字段直接丢弃。
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
		for i := 0; i+1 < len(rs); i++ {
			add(string(rs[i : i+2])) // 缓冲里已无停用字，bigram 必干净
		}
	}
	for _, r := range text {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			flushCJK()
			ascii.WriteRune(r)
		case isCJK(r):
			if cnStopRunes[r] { // 停用字=切分边界，绝不入词
				flushCJK()
				continue
			}
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
//	中文专名片段：按非汉字/停用字切段，段长>=2 的连续汉字串（整段一个实体）。
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
		var buf []rune
		emit := func() {
			if len(buf) >= 2 {
				seen[string(buf)] = struct{}{}
			}
			buf = nil
		}
		for _, r := range []rune(seg) {
			if cnStopRunes[r] { // 停用字处断开，不桥接
				emit()
				continue
			}
			buf = append(buf, r)
		}
		emit()
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

// MaybeAddCausal 若文本命中因果连接词且能切出原因/结果两侧，
// 即认为 fromNode（原因侧）导致 toNode（结果侧），落一条有向 causal 边（Weight=0.7）。
// 方向由调用方按子句先后给出；纯并列/无连接词/只有连接词无两侧内容 → 不落边。
func MaybeAddCausal(g *Graph, fromNode, toNode string, text string) {
	if g == nil || fromNode == "" || toNode == "" || fromNode == toNode {
		return
	}
	if !hasAny(text, causalWords) { // 复用 systemone.go 的大小写不敏感包含判断
		return
	}
	if _, _, ok := splitCausalClauses(text); !ok { // 只有连接词、无两侧内容 → 不落
		return
	}
	g.Add(Edge{From: fromNode, To: toNode, Rel: EdgeRelCausal, Directed: true, Weight: 0.7})
}

// splitClauses 按中英文标点把整段切成子句（去空白、丢空段）。
func splitClauses(text string) []string {
	f := func(r rune) bool { return strings.ContainsRune(clauseSplitterChars, r) }
	var out []string
	for _, seg := range strings.FieldsFunc(text, f) {
		if seg = strings.TrimSpace(seg); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// stripConns 清掉子句里残留的连接词字（P0 粗切后两半更干净）。
func stripConns(s string) string {
	for _, w := range causalWords {
		s = strings.ReplaceAll(s, w, "")
	}
	return strings.TrimSpace(s)
}

// splitCausalClauses 先按标点切子句，再在含连接词的那对子句间粗切 [原因侧 | 结果侧]：
//
//	结果侧引导词（所以/因此/于是/从而/导致/使得/造成）：
//	    连接词左侧（同子句优先，空则取上一子句）= 原因侧，连接词右侧 = 结果侧。
//	    例：「今天停电，导致工厂停工一天」→ cause=今天停电 / effect=工厂停工一天。
//	原因侧引导词（因为/鉴于）：连接词右侧=原因侧，下一子句=结果侧。
//	两半都非空才 ok=true；无连接词 / 只有连接词无两侧 → ok=false。
//
// 注释：P0 是按标点+连接词位置的粗切，不做子句依存分析；P2 换模型做精确因果归因。
func splitCausalClauses(text string) (cause, effect string, ok bool) {
	clauses := splitClauses(text)
	for i, c := range clauses {
		// 结果侧引导词：取最靠左的命中
		p, pHit := -1, ""
		for _, conn := range effectLeadWords {
			if j := strings.Index(c, conn); j >= 0 && (p < 0 || j < p) {
				p, pHit = j, conn
			}
		}
		if p >= 0 {
			effect = stripConns(c[p+len(pHit):])
			cause = stripConns(c[:p])
			if cause == "" && i > 0 { // 同子句左侧为空 → 借上一子句当原因侧
				cause = stripConns(clauses[i-1])
			}
			return cause, effect, cause != "" && effect != ""
		}
		// 原因侧引导词：取最靠左的命中
		q, qHit := -1, ""
		for _, conn := range causeLeadWords {
			if j := strings.Index(c, conn); j >= 0 && (q < 0 || j < q) {
				q, qHit = j, conn
			}
		}
		if q >= 0 {
			cause = stripConns(c[q+len(qHit):])
			if cause == "" && i > 0 {
				cause = stripConns(clauses[i-1])
			}
			effect = ""
			if i+1 < len(clauses) {
				effect = stripConns(clauses[i+1])
			}
			return cause, effect, cause != "" && effect != ""
		}
	}
	return "", "", false
}
