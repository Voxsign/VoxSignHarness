// verifylint.go -- VSL-v3"  tgtapprove "   verify now(V3-01…V3-08). 
//
// rule  : docs/VSL-v3-  tgtapprove .md
//
//	§2    define  restrict    7    charseg(claim/method/evidence/threshold/
//	      counterexample/verdict_states/approver/falsifier),   i.e." done"
//	§2.1  charseg   verifyrule V3-01…V3-08
//	§4     state forbid: unverified     as ed; E0 no  forbidstopinclose 
//	§7.2  threshold: unset  allowstore (  table "   "), but     met
//
// §2.1 endseg   outboundarylimit: **close  verify, semanticneed  **. basefileonly close verify; 
// claim and  intentis   , counterexample is   isrevexample, threshold getvalueis has
//  data,       base lint      in(see VerifyBlock.Check  returnbackvalue  ). 
//
//  tgtapprove ,     dependency:  write     YAML   resolve  (   value, in listtableand
//  listtable,  /  idchar  ,   tgt , note ), onlyoverwritebaserule  outnow  state. 
//     in YAML  --  "    dependency"is  end. 
package doccontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// RuleIDs returnbackbase lint  now rulesafety ,     as V3-01…V3-08. 
func RuleIDs() []string {
	return []string{"V3-01", "V3-02", "V3-03", "V3-04", "V3-05", "V3-06", "V3-07", "V3-08"}
}

// RuleStatus is  rule   close . 
type RuleStatus string

const (
	// StatusPass tableshow  close verify ed. 
	StatusPass RuleStatus = "pass"
	// StatusFail tableshow  close verify  ed. 
	StatusFail RuleStatus = "fail"
)

// RuleResult is  rule   : pass/fail + origbecause. 
type RuleResult struct {
	Rule   string     `json:"rule"`
	Status RuleStatus `json:"status"`
	Reason string     `json:"reason"`
}

// Report is 8  rule     . 
type Report struct {
	Results []RuleResult `json:"results"`
}

// Pass   is  8  safetyed. 
func (r *Report) Pass() bool {
	return len(r.FailedRules()) == 0
}

// FailedRules returnback has fail  rule ID(by V3-01…V3-08   ). 
func (r *Report) FailedRules() []string {
	var out []string
	for _, res := range r.Results {
		if res.Status == StatusFail {
			out = append(out, res.Rule)
		}
	}
	return out
}

// Result get  rule   ; rule ID  store timereturnback value. 
func (r *Report) Result(rule string) RuleResult {
	for _, res := range r.Results {
		if res.Rule == rule {
			return res
		}
	}
	return RuleResult{}
}

// String  out class read     , thenat    time connect toorigbecause. 
func (r *Report) String() string {
	var b strings.Builder
	for _, res := range r.Results {
		fmt.Fprintf(&b, "%s %s — %s\n", res.Rule, res.Status, res.Reason)
	}
	return b.String()
}

// VerifyBlock is"  tgtapprove "beresolve after close ize  . 
//
// Owner is nower(rule   V3-07"same  in to owner charseg"),   define top  
// owner / builder / implementer charseg; Approver etc    verify:   . 
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

// Options controlneedneedout  boundary      rule(V3-04 evidence.ref is store ). 
//
// Exists  emptytime first use (  notein);  thenby Root asbaseapprove   file  on
// os.Stat; Root asemptythenbyprocesscurbeforeobj asbaseapprove. 
type Options struct {
	Root   string
	Exists func(ref string) bool
}

// Verify resolve and   seg  tgtapprove  base, evidence.ref bycurbeforeobj asbaseapproveverify. 
func Verify(text string) (*Report, error) {
	return VerifyWith(text, Options{})
}

// VerifyWith resolve and   seg  tgtapprove  base. 
func VerifyWith(text string, opts Options) (*Report, error) {
	blk, err := ParseVerify(text)
	if err != nil {
		return nil, err
	}
	return blk.Check(opts), nil
}

// ParseVerify pipe seg YAML    define  baseresolve become VerifyBlock. 
//
//  base by top  v2 charseg(purpose/priority/…); base num     verify:   . 
// if seg basebase thenis verify  ( has verify:   ), alsoconnectaccept. 
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

// Check      V3-01…V3-08. 
//
// note : Check onlyanswer"charsegclose is  rule ". §2.1 endseg semantic  
// (claim and  intentis   , counterexample is     , threshold getvalueis 
// has data)    ,    give or  type  --i.e.  8  safety pass, also  table  
// definealready "to". 
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

// verdictStates returnback"has  statelist":  voice timegetrule     [met, unverified, not_met]. 
//
// §2 write  state"  i.e. ", and §4 needrequire unverified    form table . because ifdefine  write
// verdict_states, thenby  met handle--E0 or threshold=unset time      rule  , 
//  posis"E0 no  forbidstopinclose " recv  to. 
func (b *VerifyBlock) verdictStates() []string {
	if len(b.VerdictStates) == 0 {
		return []string{"met", "unverified", "not_met"}
	}
	return b.VerdictStates
}

// ---- V3-01…V3-08 ----

// VagueWordWhitelist is V3-01    word name : rule    claim    "  word name 
// out   word", but    name , thusdefaultasempty(= has  wordallforbid). calluse   fill. 
var VagueWordWhitelist = map[string]bool{}

// vagueWords is"    "     wordkindbase(§1  state ). in by    ,   byword boundary. 
var vagueWords = []string{
	"稳定", "高效", "优秀", "良好", "快速", "流畅", "友好", "合理", "适当", "尽量",
	"足够", "完善", "健壮", "智能", "简单", "方便", "满意",
	"好", "快", "稳", "优", "强",
	"good", "fast", "stable", "robust", "efficient", "nice", "better",
	"optimal", "satisfactory", "reasonable", "sufficient", "seamless",
}

// ruleV301: claim  empty, and    word name out   word. 
func ruleV301(b *VerifyBlock) RuleResult {
	const id = "V3-01"
	claim := strings.TrimSpace(b.Claim)
	if claim == "" {
		return ruleFail(id, "claim is empty: missing a decidable declarative sentence (§2; any missing item means incomplete)")
	}
	if w, ok := findVagueWord(claim); ok {
		return ruleFail(id, fmt.Sprintf(
			"claim contains vague adjective %q: adjectives cannot define a criterion; rewrite as observable form \"under condition C the system does/does not do X\"", w))
	}
	return rulePass(id, "claim is non-empty and contains no vague adjectives")
}

// methodEnum is §2 rule   method getvalue  . 
var methodEnum = []string{"test", "replay", "measurement", "review", "audit"}

// ruleV302: method ∈   . 
func ruleV302(b *VerifyBlock) RuleResult {
	const id = "V3-02"
	m := strings.ToLower(strings.TrimSpace(b.Method))
	if m == "" {
		return ruleFail(id, "method is empty: must explicitly declare the verification method")
	}
	for _, e := range methodEnum {
		if m == e {
			return rulePass(id, fmt.Sprintf("method=%s is in the enum", e))
		}
	}
	return ruleFail(id, fmt.Sprintf("method=%q is not in enum %v", b.Method, methodEnum))
}

// ruleV303: evidence.level ∈ {E0,E1,E2,E3}, and E0      verdict_states   met. 
func ruleV303(b *VerifyBlock) RuleResult {
	const id = "V3-03"
	lv := strings.ToUpper(strings.TrimSpace(b.EvidenceLevel))
	if lv == "" {
		return ruleFail(id, "evidence.level is empty: must explicitly declare evidence level E0-E3")
	}
	if lv != "E0" && lv != "E1" && lv != "E2" && lv != "E3" {
		return ruleFail(id, fmt.Sprintf("evidence.level=%q is not in enum {E0,E1,E2,E3}", b.EvidenceLevel))
	}
	if lv == "E0" && containsFold(b.verdictStates(), "met") {
		return ruleFail(id, "evidence.level=E0 yet verdict_states allows met: E0 has no source, must not enter conclusions (§4)")
	}
	return rulePass(id, fmt.Sprintf("evidence.level=%s is valid%s", lv, e0Note(lv)))
}

func e0Note(lv string) string {
	if lv == "E0" {
		return ", and met not allowed"
	}
	return ""
}

// ruleV304: evidence.ref  empty, andreferto  store  artifact(  open   ). 
func ruleV304(b *VerifyBlock, opts Options) RuleResult {
	const id = "V3-04"
	ref := strings.TrimSpace(b.EvidenceRef)
	if ref == "" {
		return ruleFail(id, "evidence.ref is empty: evidence must anchor to an external artifact that can be reopened")
	}
	if strings.ContainsAny(ref, "<>") {
		return ruleFail(id, fmt.Sprintf("evidence.ref=%q is still a spec placeholder, not replaced by a real artifact", ref))
	}

	path := cleanRef(ref)
	if opts.Exists != nil {
		if !opts.Exists(path) {
			return ruleFail(id, fmt.Sprintf("artifact at evidence.ref=%q does not exist", ref))
		}
		return rulePass(id, fmt.Sprintf("evidence.ref=%s exists (confirmed by external parser)", path))
	}

	p := path
	if opts.Root != "" && !filepath.IsAbs(p) {
		p = filepath.Join(opts.Root, p)
	}
	if _, err := os.Stat(p); err != nil {
		return ruleFail(id, fmt.Sprintf("artifact at evidence.ref=%q cannot be opened: %v", ref, err))
	}
	return rulePass(id, fmt.Sprintf("evidence.ref=%s exists and can be opened", p))
}

// ruleV305: threshold asnumchar+  , or as unset; unset timeforbidstop  met. 
func ruleV305(b *VerifyBlock) RuleResult {
	const id = "V3-05"
	t := strings.TrimSpace(b.Threshold)
	if t == "" {
		return ruleFail(id, "threshold missing: must give number+tolerance+source, or explicitly write unset")
	}
	if strings.EqualFold(t, "unset") {
		if containsFold(b.verdictStates(), "met") {
			return ruleFail(id, "threshold=unset yet met allowed: unverified must not count as pass (§4)")
		}
		return rulePass(id, "threshold=unset and met not claimed; allowed (§7.2)")
	}
	if !containsDigit(t) {
		return ruleFail(id, fmt.Sprintf("threshold=%q is neither unset nor a number: without a threshold, adequacy cannot be judged (§1 form three)", b.Threshold))
	}
	return rulePass(id, "threshold contains a number, usable as a decidable threshold")
}

// ruleV306: counterexample    1  , and is claim    getrev. 
func ruleV306(b *VerifyBlock) RuleResult {
	const id = "V3-06"
	ce := strings.TrimSpace(b.Counterexample)
	if ce == "" {
		return ruleFail(id, "counterexample is empty: at least 1 is required")
	}
	if isSimpleNegation(ce, b.Claim) {
		return ruleFail(id, "counterexample is just the claim negated: must be written as an observable phenomenon (failure is obvious at a glance)")
	}
	return rulePass(id, "counterexample is non-empty and not a simple negation of the claim")
}

// ruleV307: approver and nower same(same  in to owner charseg). 
func ruleV307(b *VerifyBlock) RuleResult {
	const id = "V3-07"
	approver := strings.TrimSpace(b.Approver)
	if approver == "" {
		return ruleFail(id, "approver is empty: must state who judges, and it must not be the same person/agent as the implementer")
	}
	owner := strings.TrimSpace(b.Owner)
	if owner == "" {
		return ruleFail(id, "missing owner/builder fields: cannot verify whether approver is independent of implementer (fail-closed)")
	}
	if canonicalAgent(approver) == canonicalAgent(owner) {
		return ruleFail(id, fmt.Sprintf("approver=%q and implementer owner=%q are the same person/agent", approver, owner))
	}
	return rulePass(id, fmt.Sprintf("approver=%s is independent of implementer owner=%s", approver, owner))
}

// ruleV308: falsifier  empty. 
func ruleV308(b *VerifyBlock) RuleResult {
	const id = "V3-08"
	if strings.TrimSpace(b.Falsifier) == "" {
		return ruleFail(id, "falsifier is empty: must state who can falsify it and with what counterexample")
	}
	return rulePass(id, "falsifier is non-empty")
}

// ---- rule   ----

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

// containsWord   ASCII word boundary  ,    "good"  in "goodness" ofclass middle seg. 
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

// findVagueWord   claim       in name ofout   word. 
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

// negationPrefixes is"  getrev"  diffword. 
var negationPrefixes = []string{"不", "非", "无", "未", "not ", "no ", "never "}

// isSimpleNegation    counterexample is onlyis claim    getrev(but    now ). 
//
//    form:   empty andtgtpt(keep  char/numchar)after, if counterexample etcat claim, 
// oretcat"  word + claim", or claim be body  and   splitonlyhas  word ->   getrev. 
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

// normalizeText onlykeep char andnumchar( in ),   empty , tgtptand id. 
func normalizeText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// canonicalAgent   ize"  /  now", useat V3-07  to: 
//  firsttailempty ,   tail  idnote (e.g."reviewer-agent(  as nower)"),   write,  empty . 
func canonicalAgent(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "（("); i >= 0 {
		s = s[:i]
	}
	return strings.Join(strings.Fields(strings.ToLower(s)), "")
}

// cleanRef from evidence.ref     #  pt,  to  stat  path split. 
func cleanRef(ref string) string {
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		ref = ref[:i]
	}
	return strings.TrimSpace(ref)
}

// ----    YAML   resolve   ----

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

// lexYAML pipe base become     ,   note andempty ; only keepempty   . 
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
			return nil, fmt.Errorf("line %d uses tab indentation: this parser supports space indentation only", i+1)
		}
		out = append(out, yline{
			indent: indent,
			text:   strings.TrimRight(line[indent:], " \t"),
			num:    i + 1,
		})
	}
	return out, nil
}

// stripComment    tailnote : onlyhascur # beforefaceisempty and   idintimeonly disconnect. 
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

// splitKey pipe  " : value"  open;    is ASCII tgt  ,  idafter isempty or tail. 
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
			return nil, i, fmt.Errorf("line %d indentation unexpected: expected %d spaces, got %d", ln.num, indent, ln.indent)
		}
		key, rest, ok := splitKey(ln.text)
		if !ok {
			return nil, i, fmt.Errorf("line %d is not a valid \"key: value\": %q", ln.num, ln.text)
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
				return nil, i, fmt.Errorf("line %d inline list parse failed: %v", ln.num, err)
			}
			m[key] = &yvalue{isList: true, list: items}
			continue
		}

		raw := rest
		//   tgt :   change , and isnew   ,  connecttocurbeforevalue. 
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
		return nil, fmt.Errorf("list does not start with [")
	}
	end := strings.LastIndex(s, "]")
	if end < 0 {
		return nil, fmt.Errorf("list is missing ]")
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
