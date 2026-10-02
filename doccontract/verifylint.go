// verifylint.go —— VSL-v3「验证标准层」的机器校验实现（V3-01…V3-08）。
//
// 规格书：docs/VSL-v3-验证标准层.md
//
//	§2    定义块强制追加的 7 个验证字段（claim/method/evidence/threshold/
//	      counterexample/verdict_states/approver/falsifier），缺一即"未完成"
//	§2.1  字段级机器校验规则 V3-01…V3-08
//	§4    三态门禁：unverified 不得折算为通过；E0 无来源禁止入结论
//	§7.2  threshold: unset 允许存在（诚实表达"未验证"），但不得伴随 met
//
// §2.1 末段明确划出了界限：**结构可校验，语义需评审**。本文件只做结构校验；
// claim 与真实意图是否一致、counterexample 是否真的是反例、threshold 取值是否有
// 依据，这三件事不在本 lint 的能力范围内（见 VerifyBlock.Check 的返回值说明）。
//
// 纯标准库、零第三方依赖：手写一个最小 YAML 子集解析器（缩进键值、内联列表与
// 块列表、单/双引号字符串、折行标量、注释），只覆盖本规格书出现的形态。
// 刻意不引入 YAML 库——仓库"零第三方依赖"是硬约束。
package doccontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// RuleIDs 返回本 lint 实现的规则全集，顺序固定为 V3-01…V3-08。
func RuleIDs() []string {
	return []string{"V3-01", "V3-02", "V3-03", "V3-04", "V3-05", "V3-06", "V3-07", "V3-08"}
}

// RuleStatus 是一条规则的裁定结果。
type RuleStatus string

const (
	// StatusPass 表示该条结构校验通过。
	StatusPass RuleStatus = "pass"
	// StatusFail 表示该条结构校验不通过。
	StatusFail RuleStatus = "fail"
)

// RuleResult 是单条规则的裁定：pass/fail + 原因。
type RuleResult struct {
	Rule   string     `json:"rule"`
	Status RuleStatus `json:"status"`
	Reason string     `json:"reason"`
}

// Report 是 8 条规则的裁定集合。
type Report struct {
	Results []RuleResult `json:"results"`
}

// Pass 报告是否 8 条全过。
func (r *Report) Pass() bool {
	return len(r.FailedRules()) == 0
}

// FailedRules 返回所有 fail 的规则 ID（按 V3-01…V3-08 顺序）。
func (r *Report) FailedRules() []string {
	var out []string
	for _, res := range r.Results {
		if res.Status == StatusFail {
			out = append(out, res.Rule)
		}
	}
	return out
}

// Result 取某条规则的裁定；规则 ID 不存在时返回零值。
func (r *Report) Result(rule string) RuleResult {
	for _, res := range r.Results {
		if res.Rule == rule {
			return res
		}
	}
	return RuleResult{}
}

// String 输出人类可读的裁定明细，便于测试失败时直接看到原因。
func (r *Report) String() string {
	var b strings.Builder
	for _, res := range r.Results {
		fmt.Fprintf(&b, "%s %s — %s\n", res.Rule, res.Status, res.Reason)
	}
	return b.String()
}

// VerifyBlock 是"验证标准块"被解析后的结构化视图。
//
// Owner 是实现者（规格书 V3-07「同仓库内比对 owner 字段」），来自定义块顶层的
// owner / builder / implementer 字段；Approver 等人来自 verify: 子块。
type VerifyBlock struct {
	Claim          string
	Method         string
	EvidenceLevel  string
	EvidenceKind   string
	EvidenceRef    string
	Threshold      string
	Counterexample string
	VerdictStates  []string
	Approver       string
	Falsifier      string
	Owner          string
}

// Options 控制需要外部世界信息的那一条规则（V3-04 evidence.ref 是否存在）。
//
// Exists 非空时优先使用它（测试注入）；否则以 Root 为基准在真实文件系统上
// os.Stat；Root 为空则以进程当前目录为基准。
type Options struct {
	Root   string
	Exists func(ref string) bool
}

// Verify 解析并裁定一段验证标准块文本，evidence.ref 以当前目录为基准校验。
func Verify(text string) (*Report, error) {
	return VerifyWith(text, Options{})
}

// VerifyWith 解析并裁定一段验证标准块文本。
func VerifyWith(text string, opts Options) (*Report, error) {
	blk, err := ParseVerify(text)
	if err != nil {
		return nil, err
	}
	return blk.Check(opts), nil
}

// ParseVerify 把一段 YAML 风格的定义块文本解析成 VerifyBlock。
//
// 文本可以带顶层 v2 字段（purpose/priority/…）；本函数自动定位 verify: 子块。
// 若整段文本本身就是 verify 块（没有 verify: 包裹），也接受。
func ParseVerify(text string) (*VerifyBlock, error) {
	lines, err := lexYAML(text)
	if err != nil {
		return nil, err
	}

	root := map[string]*yvalue{}
	if len(lines) > 0 {
		root, _, err = parseYAMLMap(lines, 0, lines[0].indent)
		if err != nil {
			return nil, err
		}
	}

	blk := &VerifyBlock{}
	vb := root["verify"]
	if vb == nil || !vb.isMap {
		if looksLikeVerify(root) {
			vb = &yvalue{isMap: true, fields: root}
		}
	}
	if vb != nil && vb.isMap {
		f := vb.fields
		blk.Claim = scalarOf(f["claim"])
		blk.Method = scalarOf(f["method"])
		blk.Threshold = scalarOf(f["threshold"])
		blk.Counterexample = scalarOf(f["counterexample"])
		blk.Approver = scalarOf(f["approver"])
		blk.Falsifier = scalarOf(f["falsifier"])
		blk.VerdictStates = listOf(f["verdict_states"])
		if ev := f["evidence"]; ev != nil && ev.isMap {
			blk.EvidenceLevel = scalarOf(ev.fields["level"])
			blk.EvidenceKind = scalarOf(ev.fields["kind"])
			blk.EvidenceRef = scalarOf(ev.fields["ref"])
		}
	}

	blk.Owner = firstScalar(root, "owner", "builder", "implementer")
	return blk, nil
}

// Check 逐条裁定 V3-01…V3-08。
//
// 注意：Check 只回答"字段结构是否合规格"。§2.1 末段的语义问题
// （claim 与真实意图是否一致、counterexample 是否真能证伪、threshold 取值是否
// 有依据）一律判不了，必须留给人或强模型评审——即使 8 条全 pass，也不代表这条
// 定义已经"对了"。
func (b *VerifyBlock) Check(opts Options) *Report {
	return &Report{Results: []RuleResult{
		ruleV301(b),
		ruleV302(b),
		ruleV303(b),
		ruleV304(b, opts),
		ruleV305(b),
		ruleV306(b),
		ruleV307(b),
		ruleV308(b),
	}}
}

// verdictStates 返回"有效三态清单"：未声明时取规格书缺省 [met, unverified, not_met]。
//
// §2 写明三态"缺省即此"，且 §4 要求 unverified 必须显式可表达。因此若定义块没写
// verdict_states，就按含 met 处理——E0 或 threshold=unset 时这会导致相应规则失败，
// 这正是"E0 无来源禁止入结论"的收紧方向。
func (b *VerifyBlock) verdictStates() []string {
	if len(b.VerdictStates) == 0 {
		return []string{"met", "unverified", "not_met"}
	}
	return b.VerdictStates
}

// ---- V3-01…V3-08 ----

// VagueWordWhitelist 是 V3-01 的形容词白名单：规格书说 claim 不得含"形容词白名单
// 外的模糊词"，但未枚举白名单，故默认为空（=所有模糊词都禁）。调用方可扩充。
var VagueWordWhitelist = map[string]bool{}

// vagueWords 是"不可判定"的模糊形容词样本（§1 形态一）。中文按子串匹配，英文按词边界。
var vagueWords = []string{
	"稳定", "高效", "优秀", "良好", "快速", "流畅", "友好", "合理", "适当", "尽量",
	"足够", "完善", "健壮", "智能", "简单", "方便", "满意",
	"好", "快", "稳", "优", "强",
	"good", "fast", "stable", "robust", "efficient", "nice", "better",
	"optimal", "satisfactory", "reasonable", "sufficient", "seamless",
}

// ruleV301：claim 非空，且不含形容词白名单外的模糊词。
func ruleV301(b *VerifyBlock) RuleResult {
	const id = "V3-01"
	claim := strings.TrimSpace(b.Claim)
	if claim == "" {
		return ruleFail(id, "claim 为空：缺少可判定的陈述句（§2 缺一即未完成）")
	}
	if w, ok := findVagueWord(claim); ok {
		return ruleFail(id, fmt.Sprintf(
			"claim 含模糊形容词 %q：形容词无法构造判据，须改成「在条件 C 下系统做/不做 X」的可观测形式", w))
	}
	return rulePass(id, "claim 非空，且不含模糊形容词")
}

// methodEnum 是 §2 规定的 method 取值集合。
var methodEnum = []string{"test", "replay", "measurement", "review", "audit"}

// ruleV302：method ∈ 枚举。
func ruleV302(b *VerifyBlock) RuleResult {
	const id = "V3-02"
	m := strings.ToLower(strings.TrimSpace(b.Method))
	if m == "" {
		return ruleFail(id, "method 为空：必须显式声明验证方式")
	}
	for _, e := range methodEnum {
		if m == e {
			return rulePass(id, fmt.Sprintf("method=%s 在枚举内", e))
		}
	}
	return ruleFail(id, fmt.Sprintf("method=%q 不在枚举 %v 内", b.Method, methodEnum))
}

// ruleV303：evidence.level ∈ {E0,E1,E2,E3}，且 E0 不得伴随 verdict_states 含 met。
func ruleV303(b *VerifyBlock) RuleResult {
	const id = "V3-03"
	lv := strings.ToUpper(strings.TrimSpace(b.EvidenceLevel))
	if lv == "" {
		return ruleFail(id, "evidence.level 为空：必须显式声明证据等级 E0–E3")
	}
	if lv != "E0" && lv != "E1" && lv != "E2" && lv != "E3" {
		return ruleFail(id, fmt.Sprintf("evidence.level=%q 不在枚举 {E0,E1,E2,E3} 内", b.EvidenceLevel))
	}
	if lv == "E0" && containsFold(b.verdictStates(), "met") {
		return ruleFail(id, "evidence.level=E0 却允许 verdict_states 含 met：E0 无来源，禁止入结论（§4）")
	}
	return rulePass(id, fmt.Sprintf("evidence.level=%s 合法%s", lv, e0Note(lv)))
}

func e0Note(lv string) string {
	if lv == "E0" {
		return "，且未允许判 met"
	}
	return ""
}

// ruleV304：evidence.ref 非空，且指向真实存在的产物（可 open 验证）。
func ruleV304(b *VerifyBlock, opts Options) RuleResult {
	const id = "V3-04"
	ref := strings.TrimSpace(b.EvidenceRef)
	if ref == "" {
		return ruleFail(id, "evidence.ref 为空：证据必须锚定到可重新打开的外部产物")
	}
	if strings.ContainsAny(ref, "<>") {
		return ruleFail(id, fmt.Sprintf("evidence.ref=%q 仍是规格书占位符，未替换为真实产物", ref))
	}

	path := cleanRef(ref)
	if opts.Exists != nil {
		if !opts.Exists(path) {
			return ruleFail(id, fmt.Sprintf("evidence.ref=%q 指向的产物不存在", ref))
		}
		return rulePass(id, fmt.Sprintf("evidence.ref=%s 存在（外部解析器确认）", path))
	}

	p := path
	if opts.Root != "" && !filepath.IsAbs(p) {
		p = filepath.Join(opts.Root, p)
	}
	if _, err := os.Stat(p); err != nil {
		return ruleFail(id, fmt.Sprintf("evidence.ref=%q 指向的产物无法打开: %v", ref, err))
	}
	return rulePass(id, fmt.Sprintf("evidence.ref=%s 存在且可打开", p))
}

// ruleV305：threshold 为数字+单位，或恰为 unset；unset 时禁止判 met。
func ruleV305(b *VerifyBlock) RuleResult {
	const id = "V3-05"
	t := strings.TrimSpace(b.Threshold)
	if t == "" {
		return ruleFail(id, "threshold 缺失：必须给数字+容差+出处，或显式写 unset")
	}
	if strings.EqualFold(t, "unset") {
		if containsFold(b.verdictStates(), "met") {
			return ruleFail(id, "threshold=unset 却允许判 met：未验证不得折算为通过（§4）")
		}
		return rulePass(id, "threshold=unset 且未声称 met，允许存在（§7.2）")
	}
	if !containsDigit(t) {
		return ruleFail(id, fmt.Sprintf("threshold=%q 既不是 unset 也不含数字：无阈值无法判够不够（§1 形态三）", b.Threshold))
	}
	return rulePass(id, "threshold 含数字，可作为可判定阈值")
}

// ruleV306：counterexample 至少 1 条，且不是 claim 的简单取反。
func ruleV306(b *VerifyBlock) RuleResult {
	const id = "V3-06"
	ce := strings.TrimSpace(b.Counterexample)
	if ce == "" {
		return ruleFail(id, "counterexample 为空：至少须有 1 条")
	}
	if isSimpleNegation(ce, b.Claim) {
		return ruleFail(id, "counterexample 只是 claim 的简单取反：必须写成可观测现象（错了一看就知道）")
	}
	return rulePass(id, "counterexample 非空，且不是 claim 的简单取反")
}

// ruleV307：approver 与实现者不同（同仓库内比对 owner 字段）。
func ruleV307(b *VerifyBlock) RuleResult {
	const id = "V3-07"
	approver := strings.TrimSpace(b.Approver)
	if approver == "" {
		return ruleFail(id, "approver 为空：必须写明谁判，且不得与实现者同一人/同一 agent")
	}
	owner := strings.TrimSpace(b.Owner)
	if owner == "" {
		return ruleFail(id, "缺少 owner/builder 字段：无法比对 approver 是否独立于实现者（fail-closed）")
	}
	if canonicalAgent(approver) == canonicalAgent(owner) {
		return ruleFail(id, fmt.Sprintf("approver=%q 与实现者 owner=%q 是同一人/同一 agent", approver, owner))
	}
	return rulePass(id, fmt.Sprintf("approver=%s 独立于实现者 owner=%s", approver, owner))
}

// ruleV308：falsifier 非空。
func ruleV308(b *VerifyBlock) RuleResult {
	const id = "V3-08"
	if strings.TrimSpace(b.Falsifier) == "" {
		return ruleFail(id, "falsifier 为空：必须写明谁能证伪它、用什么反例")
	}
	return rulePass(id, "falsifier 非空")
}

// ---- 规则辅助 ----

func rulePass(rule, reason string) RuleResult {
	return RuleResult{Rule: rule, Status: StatusPass, Reason: reason}
}

func ruleFail(rule, reason string) RuleResult {
	return RuleResult{Rule: rule, Status: StatusFail, Reason: reason}
}

func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(strings.TrimSpace(s), want) {
			return true
		}
	}
	return false
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// containsWord 做 ASCII 词边界匹配，避免 "good" 命中 "goodness" 之类的中间片段。
func containsWord(s, w string) bool {
	for idx := 0; ; {
		j := strings.Index(s[idx:], w)
		if j < 0 {
			return false
		}
		start := idx + j
		end := start + len(w)
		leftOK := start == 0 || !isLetterByte(s[start-1])
		rightOK := end == len(s) || !isLetterByte(s[end])
		if leftOK && rightOK {
			return true
		}
		idx = start + 1
	}
}

func isLetterByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isASCIIWord(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// findVagueWord 在 claim 里找第一个命中白名单之外的模糊词。
func findVagueWord(claim string) (string, bool) {
	lower := strings.ToLower(claim)
	for _, w := range vagueWords {
		if VagueWordWhitelist[w] || VagueWordWhitelist[strings.ToLower(w)] {
			continue
		}
		if isASCIIWord(w) {
			if containsWord(lower, w) {
				return w, true
			}
			continue
		}
		if strings.Contains(claim, w) {
			return w, true
		}
	}
	return "", false
}

// negationPrefixes 是"简单取反"的识别词。
var negationPrefixes = []string{"不", "非", "无", "未", "not ", "no ", "never "}

// isSimpleNegation 判定 counterexample 是否只是 claim 的简单取反（而非可观测现象）。
//
// 判定方式：去掉空白与标点（保留文字/数字）后，若 counterexample 等于 claim，
// 或等于"否定词 + claim"，或 claim 被整体包含且多余部分只有否定词 → 简单取反。
func isSimpleNegation(counterexample, claim string) bool {
	nc := normalizeText(claim)
	if nc == "" {
		return false
	}
	n := normalizeText(counterexample)
	if n == nc {
		return true
	}
	if strings.Contains(n, nc) {
		rest := strings.Replace(n, nc, "", 1)
		if onlyNegation(rest) {
			return true
		}
	}
	return false
}

func onlyNegation(s string) bool {
	s = normalizeText(s)
	for s != "" {
		matched := false
		for _, p := range negationPrefixes {
			np := normalizeText(p)
			if np == "" {
				continue
			}
			if strings.HasPrefix(s, np) {
				s = strings.TrimPrefix(s, np)
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// normalizeText 只保留字母与数字（含中文），丢掉空白、标点与引号。
func normalizeText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// canonicalAgent 归一化"谁判/谁实现"，用于 V3-07 比对：
// 去首尾空白、丢掉尾随括号注释（如「reviewer-agent（不得为实现者）」）、转小写、去空格。
func canonicalAgent(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "（("); i >= 0 {
		s = s[:i]
	}
	return strings.Join(strings.Fields(strings.ToLower(s)), "")
}

// cleanRef 从 evidence.ref 里去掉 # 锚点，得到可 stat 的路径部分。
func cleanRef(ref string) string {
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		ref = ref[:i]
	}
	return strings.TrimSpace(ref)
}

// ---- 最小 YAML 子集解析器 ----

type yline struct {
	indent int
	text   string
	num    int
}

type yvalue struct {
	scalar string
	list   []string
	fields map[string]*yvalue
	isList bool
	isMap  bool
}

// lexYAML 把文本切成带缩进的行，去掉注释与空行；只支持空格缩进。
func lexYAML(text string) ([]yline, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out []yline
	for i, raw := range strings.Split(text, "\n") {
		line := stripComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := 0
		for indent < len(line) && line[indent] == ' ' {
			indent++
		}
		if indent < len(line) && line[indent] == '\t' {
			return nil, fmt.Errorf("第 %d 行使用了制表符缩进：本解析器只支持空格缩进", i+1)
		}
		out = append(out, yline{
			indent: indent,
			text:   strings.TrimRight(line[indent:], " \t"),
			num:    i + 1,
		})
	}
	return out, nil
}

// stripComment 去掉行尾注释：只有当 # 前面是空白且不在引号内时才截断。
func stripComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '#':
			if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
				return s[:i]
			}
		}
	}
	return s
}

// splitKey 把一个"键: 值"行拆开；键必须是 ASCII 标识符，冒号后须是空格或行尾。
func splitKey(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == ':':
			if i+1 < len(s) && s[i+1] != ' ' {
				continue
			}
			k := strings.TrimSpace(s[:i])
			if !validKey(k) {
				return "", "", false
			}
			return k, strings.TrimSpace(s[i+1:]), true
		case s[i] == ' ' || s[i] == '\t':
			return "", "", false
		}
	}
	return "", "", false
}

func validKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		if unicode.IsLetter(r) || r == '_' {
			continue
		}
		if i > 0 && (unicode.IsDigit(r) || r == '-' || r == '.') {
			continue
		}
		return false
	}
	return true
}

func parseYAMLMap(lines []yline, i, indent int) (map[string]*yvalue, int, error) {
	m := map[string]*yvalue{}
	for i < len(lines) {
		ln := lines[i]
		if ln.indent < indent {
			break
		}
		if ln.indent > indent {
			return nil, i, fmt.Errorf("第 %d 行缩进异常：期望 %d 个空格，实际 %d", ln.num, indent, ln.indent)
		}
		key, rest, ok := splitKey(ln.text)
		if !ok {
			return nil, i, fmt.Errorf("第 %d 行不是合法的「键: 值」：%q", ln.num, ln.text)
		}
		i++

		if rest == "" {
			if i < len(lines) && lines[i].indent > indent {
				child, ni, err := parseYAMLValue(lines, i, lines[i].indent)
				if err != nil {
					return nil, i, err
				}
				m[key] = child
				i = ni
			} else {
				m[key] = &yvalue{scalar: ""}
			}
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(rest), "[") {
			items, err := parseInlineList(rest)
			if err != nil {
				return nil, i, fmt.Errorf("第 %d 行内联列表解析失败: %v", ln.num, err)
			}
			m[key] = &yvalue{isList: true, list: items}
			continue
		}

		raw := rest
		// 折行标量：缩进更深、且不是新键的行，拼接到当前值。
		for i < len(lines) && lines[i].indent > indent {
			if _, _, isKey := splitKey(lines[i].text); isKey {
				break
			}
			raw += " " + strings.TrimSpace(lines[i].text)
			i++
		}
		m[key] = &yvalue{scalar: unquote(raw)}
	}
	return m, i, nil
}

func parseYAMLValue(lines []yline, i, indent int) (*yvalue, int, error) {
	t := strings.TrimSpace(lines[i].text)
	if t == "-" || strings.HasPrefix(t, "- ") {
		var items []string
		for i < len(lines) && lines[i].indent >= indent {
			item := strings.TrimSpace(lines[i].text)
			if item == "-" {
				items = append(items, "")
				i++
				continue
			}
			if !strings.HasPrefix(item, "- ") {
				break
			}
			items = append(items, unquote(strings.TrimSpace(item[2:])))
			i++
		}
		return &yvalue{isList: true, list: items}, i, nil
	}
	m, ni, err := parseYAMLMap(lines, i, indent)
	if err != nil {
		return nil, i, err
	}
	return &yvalue{isMap: true, fields: m}, ni, nil
}

func parseInlineList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return nil, fmt.Errorf("不是以 [ 开头的列表")
	}
	end := strings.LastIndex(s, "]")
	if end < 0 {
		return nil, fmt.Errorf("列表缺少 ]")
	}
	body := strings.TrimSpace(s[1:end])
	if body == "" {
		return nil, nil
	}
	var out []string
	for _, p := range strings.Split(body, ",") {
		if v := unquote(strings.TrimSpace(p)); v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func scalarOf(v *yvalue) string {
	if v == nil {
		return ""
	}
	if v.isList {
		return strings.Join(v.list, ",")
	}
	return strings.TrimSpace(v.scalar)
}

func listOf(v *yvalue) []string {
	if v == nil {
		return nil
	}
	if v.isList {
		return v.list
	}
	if s := strings.TrimSpace(v.scalar); s != "" {
		return []string{s}
	}
	return nil
}

func looksLikeVerify(m map[string]*yvalue) bool {
	for _, k := range []string{
		"claim", "method", "evidence", "threshold",
		"counterexample", "verdict_states", "approver", "falsifier",
	} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func firstScalar(m map[string]*yvalue, keys ...string) string {
	for _, k := range keys {
		if s := scalarOf(m[k]); s != "" {
			return s
		}
	}
	return ""
}
