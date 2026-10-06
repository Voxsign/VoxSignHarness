//go:build vhsplan

// sm_selfmodel_test.go -- VHS-SELFPROJ-001 §2①:    type(SM-1..SM-5). 
//
// be  : harness   produce  obj type eval/projmodel/harness-*.json(VHS-PROJMODEL-001  form). 
//
//  value  (diff : basefilefrom    **  heavynew  **,   import eval/projmodel/gen  close ): 
//
//	   ← tools.LoadContracts(dir).All()(  name +   cap risk + allowed_spaces)
//	intent ← contract/contract.go   Intent*    × occurproduce code use
//	       ( path:   _test.go,   contract  inoutnow contract.IntentX; and SK-5 same )
//	domain   ← space.Load(dir).List()/Get()(perm/risk_default/wordtable)
//	close  ←   file   / go list / git
//
//  data: 
//
//	SM-1     all referto value(source     to  on file)
//	SM-2  toto : voice − value=  ;  value−voice =  (empty type    ,  allowemptyed)
//	SM-3  state  : verified/inferred/unknown; unknown need as  ; safety verified =   
//	SM-4 verified       and code  ( codechange type change ->  )
//	SM-5 occurproduce  side(plan.ExportManifest)and typeartifact  giveoutsame   value(  now  )
//
//    dataall  ** torevexample  **: first      notein  , again  artifact. 
//   : go test -tags vhsplan ./plan -run TestSM -v
package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// ----------  typeartifact  status(VHS-PROJMODEL-001 §2) ----------

type pmSubject struct {
	Repo         string `json:"repo"`
	Commit       string `json:"commit"`
	ProducedBy   string `json:"produced_by"`
	ProducedAt   string `json:"produced_at"`
	Independence string `json:"independence"`
}

type pmCapability struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Source       string   `json:"source"`
	Status       string   `json:"status"`
	Limits       []string `json:"limits"`
	NeedsConfirm bool     `json:"needs_confirm"`
	Note         string   `json:"note"`
}

type pmDormant struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Status string `json:"status"`
	Why    string `json:"why"`
}

type pmUnresolved struct {
	ID     string `json:"id"`
	SeenIn string `json:"seen_in"`
	Status string `json:"status"`
	Why    string `json:"why"`
}

type pmPerms struct {
	Read  bool     `json:"read"`
	Write bool     `json:"write"`
	Exec  []string `json:"exec"`
}

type pmDomain struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Status      string   `json:"status"`
	Perms       pmPerms  `json:"perms"`
	Aliases     []string `json:"aliases"`
	RiskDefault string   `json:"risk_default"`
}

type pmBoundary struct {
	ID     string `json:"id"`
	Claim  string `json:"claim"`
	Source string `json:"source"`
	Status string `json:"status"`
}

type pmDependency struct {
	On     string          `json:"on"`
	Kind   string          `json:"kind"`
	For    string          `json:"for"`
	Status string          `json:"status"`
	Note   string          `json:"note"`
	Source json.RawMessage `json:"source"`
}

type pmState struct {
	Tests    map[string]string `json:"tests"`
	Branches []string          `json:"branches"`
	Source   string            `json:"source"`
	Status   string            `json:"status"`
	Note     string            `json:"note"`
}

type pmQuestion struct {
	Q            string `json:"q"`
	WhyItMatters string `json:"why_it_matters"`
	Status       string `json:"status"`
}

type pmModel struct {
	SchemaVersion string         `json:"schema_version"`
	Subject       pmSubject      `json:"subject"`
	Capabilities  []pmCapability `json:"capabilities"`
	Dormant       []pmDormant    `json:"dormant"`
	Unresolved    []pmUnresolved `json:"unresolved"`
	Domains       []pmDomain     `json:"domains"`
	Boundaries    []pmBoundary   `json:"boundaries"`
	Dependencies  []pmDependency `json:"dependencies"`
	State         pmState        `json:"state"`
	OpenQuestions []pmQuestion   `json:"open_questions"`
}

// pmLoad    harness side typeartifact;   tothenstop(  data !=  ed). 
func pmLoad(t *testing.T) (string, pmModel) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(scRepoRoot(t), "eval", "projmodel", "harness-*.json"))
	if err != nil {
		t.Fatalf("[防空过] 模型产物查找失败: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("[防空过] 没有 eval/projmodel/harness-*.json → 无产物可对账（缺证据≠通过）")
	}
	sort.Strings(matches)
	path := matches[len(matches)-1]
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[防空过] 模型产物读不到 %s: %v", path, err)
	}
	var m pmModel
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("[防空过] 模型产物解析失败 %s: %v", path, err)
	}
	if len(m.Capabilities) == 0 {
		t.Fatalf("[防空过] %s 的 capabilities 为空 → 双向对账会假绿", path)
	}
	return path, m
}

// ----------  code value(    ,   use out  close ) ----------

type pmDomainTruth struct {
	Read        bool
	Write       bool
	Exec        []string
	RiskDefault string
	Aliases     []string
}

type pmTruth struct {
	ToolNames      map[string]bool
	ToolCaps       map[string]string // "file.read" -> risk
	LiveIntents    map[string]bool
	DormantIntents map[string]bool
	Domains        map[string]pmDomainTruth
	AliasUnion     map[string]bool
}

var (
	pmIntentDecl = regexp.MustCompile(`(Intent[A-Za-z]+)\s*=\s*"([A-Z_]+)"`)
	pmIntentUse  = regexp.MustCompile(`contract\.(Intent[A-Za-z]+)`)
)

func pmDeriveTruth(t *testing.T) pmTruth {
	t.Helper()
	root := scRepoRoot(t)
	tr, sr := loadRegistries(t)
	truth := pmTruth{
		ToolNames:      map[string]bool{},
		ToolCaps:       map[string]string{},
		LiveIntents:    map[string]bool{},
		DormantIntents: map[string]bool{},
		Domains:        map[string]pmDomainTruth{},
		AliasUnion:     map[string]bool{},
	}
	for _, c := range tr.All() {
		truth.ToolNames[c.Name] = true
		for _, cp := range c.Caps {
			truth.ToolCaps[c.Name+"."+cp] = c.Risk[cp]
		}
	}
	if len(truth.ToolCaps) == 0 {
		t.Fatal("[防空过] 真值侧导出为空 → 对账无从判定")
	}

	// intent:   voice  × occurproduce code use( pathsame SK-5). 
	src, err := os.ReadFile(filepath.Join(root, "contract", "contract.go"))
	if err != nil {
		t.Fatalf("[防空过] 读不到 contract/contract.go: %v", err)
	}
	declared := map[string]string{}
	for _, m := range pmIntentDecl.FindAllStringSubmatch(string(src), -1) {
		declared[m[1]] = m[2]
	}
	if len(declared) == 0 {
		t.Fatal("[防空过] 意图常量解析为空 → 对账无从判定")
	}
	referenced := map[string]bool{}
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "dist", "node_modules", "doc-fetch-resources":
				return fs.SkipDir
			}
			if d.Name() == "contract" && filepath.Dir(p) == root {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, mm := range pmIntentUse.FindAllStringSubmatch(string(b), -1) {
			referenced[mm[1]] = true
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("[防空过] 生产代码扫描失败: %v", walkErr)
	}
	for name, val := range declared {
		if referenced[name] {
			truth.LiveIntents[val] = true
		} else {
			truth.DormantIntents[val] = true
		}
	}

	// domain + domain diffname(wordtable  is   nameword). 
	for _, name := range sr.List() {
		man, ok := sr.Get(name)
		if !ok {
			continue
		}
		dt := pmDomainTruth{
			Read:        man.Perms.Read,
			Write:       man.Perms.Write,
			Exec:        append([]string{}, man.Perms.Exec...),
			RiskDefault: man.RiskDefault,
		}
		for _, tl := range man.Tools {
			if truth.ToolNames[tl] {
				continue
			}
			dt.Aliases = append(dt.Aliases, tl)
			truth.AliasUnion[tl] = true
		}
		sort.Strings(dt.Aliases)
		truth.Domains[name] = dt
	}
	if len(truth.Domains) == 0 {
		t.Fatal("[防空过] 真值侧域集合为空 → 对账无从判定")
	}
	return truth
}

// ----------  toto (SM-2     state) ----------

func pmLimitRisk(limits []string, capName string) string {
	prefix := capName + ":"
	for _, l := range limits {
		if strings.HasPrefix(l, prefix) {
			return strings.TrimPrefix(l, prefix)
		}
	}
	return ""
}

func pmSameStrings(a, b []string) bool {
	as := append([]string{}, a...)
	bs := append([]string{}, b...)
	sort.Strings(as)
	sort.Strings(bs)
	if len(as) != len(bs) {
		return false
	}
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

// pmReconcile returnback" type vs  code value" safety split :   (voice − value)and  ( value−voice ). 
func pmReconcile(m pmModel, truth pmTruth) []string {
	var out []string
	if len(m.Capabilities) == 0 {
		return append(out, "[防空过] capabilities 为空 → 双向对账在空输入上必然假绿")
	}

	modelCaps := map[string]bool{}
	modelTools := map[string]bool{}
	for _, c := range m.Capabilities {
		if c.Kind != "tool" {
			continue
		}
		modelCaps[c.ID] = true
		if i := strings.Index(c.ID, "."); i > 0 {
			modelTools[c.ID[:i]] = true
		} else {
			modelTools[c.ID] = true
		}
	}
	for id := range modelCaps {
		if _, ok := truth.ToolCaps[id]; !ok {
			out = append(out, fmt.Sprintf("[虚报] 能力 %q 在契约注册表里不存在", id))
		}
	}
	for id := range truth.ToolCaps {
		if !modelCaps[id] {
			out = append(out, fmt.Sprintf("[隐瞒] 契约能力 %q 没进模型", id))
		}
	}
	for n := range modelTools {
		if !truth.ToolNames[n] {
			out = append(out, fmt.Sprintf("[虚报] 工具 %q 不是契约", n))
		}
	}
	for n := range truth.ToolNames {
		if !modelTools[n] {
			out = append(out, fmt.Sprintf("[隐瞒] 契约 %q 没进模型", n))
		}
	}
	for _, c := range m.Capabilities {
		if c.Kind != "tool" {
			continue
		}
		want, ok := truth.ToolCaps[c.ID]
		if !ok {
			continue // already   
		}
		i := strings.Index(c.ID, ".")
		if i <= 0 {
			continue
		}
		if got := pmLimitRisk(c.Limits, c.ID[i+1:]); got != want {
			out = append(out, fmt.Sprintf("[虚报] %s 的 risk=%q 与契约不符（真值 %q）", c.ID, got, want))
		}
	}

	live := map[string]bool{}
	for _, c := range m.Capabilities {
		if c.Kind == "intent" {
			live[c.ID] = true
		}
	}
	for v := range live {
		if !truth.LiveIntents[v] {
			out = append(out, fmt.Sprintf("[虚报] 意图 %s 被当成 live（生产代码无引用）", v))
		}
	}
	for v := range truth.LiveIntents {
		if !live[v] {
			out = append(out, fmt.Sprintf("[隐瞒] live 意图 %s 没进 capabilities", v))
		}
	}
	dorm := map[string]bool{}
	for _, d := range m.Dormant {
		dorm[d.ID] = true
	}
	for v := range dorm {
		if !truth.DormantIntents[v] {
			out = append(out, fmt.Sprintf("[虚报] %s 不是 dormant", v))
		}
		if live[v] {
			out = append(out, fmt.Sprintf("[矛盾] %s 同时是 live 与 dormant", v))
		}
	}
	for v := range truth.DormantIntents {
		if !dorm[v] {
			out = append(out, fmt.Sprintf("[隐瞒] dormant 意图 %s 没登记（漏报会导致以为自己做不到）", v))
		}
	}

	modelDomains := map[string]pmDomain{}
	for _, d := range m.Domains {
		modelDomains[d.Name] = d
	}
	for name := range modelDomains {
		if _, ok := truth.Domains[name]; !ok {
			out = append(out, fmt.Sprintf("[虚报] 域 %q 不存在", name))
		}
	}
	for name, dt := range truth.Domains {
		md, ok := modelDomains[name]
		if !ok {
			out = append(out, fmt.Sprintf("[隐瞒] 域 %q 没进模型", name))
			continue
		}
		if !pmSameStrings(md.Aliases, dt.Aliases) {
			out = append(out, fmt.Sprintf("[对账] 域 %s 的别名不符: 模型 %v vs 真值 %v", name, md.Aliases, dt.Aliases))
		}
		if md.Perms.Read != dt.Read || md.Perms.Write != dt.Write || !pmSameStrings(md.Perms.Exec, dt.Exec) {
			out = append(out, fmt.Sprintf("[对账] 域 %s 的 perms 不符: 模型 %+v vs 真值 read=%v write=%v exec=%v",
				name, md.Perms, dt.Read, dt.Write, dt.Exec))
		}
		if md.RiskDefault != dt.RiskDefault {
			out = append(out, fmt.Sprintf("[对账] 域 %s 的 risk_default 不符: %q vs %q", name, md.RiskDefault, dt.RiskDefault))
		}
	}

	unres := map[string]bool{}
	for _, u := range m.Unresolved {
		unres[u.ID] = true
	}
	for id := range unres {
		if !truth.AliasUnion[id] {
			out = append(out, fmt.Sprintf("[虚报] unresolved %q 不是域级别名", id))
		}
	}
	for id := range truth.AliasUnion {
		if !unres[id] {
			out = append(out, fmt.Sprintf("[隐瞒] 域级别名 %q 既没进 unresolved 也没进 capabilities（丢弃即隐瞒）", id))
		}
	}
	return out
}

func pmHasViolation(violations []string, substr string) bool {
	for _, v := range violations {
		if strings.Contains(v, substr) {
			return true
		}
	}
	return false
}

// ---------- SM-1:  valuerefer    ----------

var pmPathToken = regexp.MustCompile(`[\p{Han}\w./-]+\.(?:go|md|json|jsonl|mod)`)

func pmSourceTokens(s string) []string {
	var out []string
	for _, tok := range pmPathToken.FindAllString(s, -1) {
		if i := strings.Index(tok, ":"); i > 0 {
			tok = tok[:i]
		}
		out = append(out, tok)
	}
	return out
}

// pmUnresolvableSources recv "source referto store  file /  state   / kind   "   . 
func pmUnresolvableSources(root string, m pmModel) []string {
	var out []string
	check := func(section, id, source string) {
		if strings.TrimSpace(source) == "" {
			out = append(out, fmt.Sprintf("[SM-1] %s %s 没有 source（每条陈述都要能指到真值）", section, id))
			return
		}
		toks := pmSourceTokens(source)
		if len(toks) == 0 {
			out = append(out, fmt.Sprintf("[SM-1] %s %s 的 source=%q 里没有可核的文件指针", section, id, source))
			return
		}
		for _, tok := range toks {
			if _, err := os.Stat(filepath.Join(root, tok)); err != nil {
				out = append(out, fmt.Sprintf("[SM-1] %s %s 的 source=%q 指向不存在的文件 %s", section, id, source, tok))
			}
		}
	}
	for _, c := range m.Capabilities {
		check("capability", c.ID, c.Source)
		switch c.Kind {
		case "tool", "intent", "command":
		default:
			out = append(out, fmt.Sprintf("[SM-1] capability %s 的 kind=%q 非法（tool|intent|command）", c.ID, c.Kind))
		}
	}
	for _, d := range m.Dormant {
		check("dormant", d.ID, d.Source)
	}
	for _, u := range m.Unresolved {
		check("unresolved", u.ID, u.SeenIn)
	}
	for _, d := range m.Domains {
		check("domain", d.Name, d.Source)
	}
	for _, b := range m.Boundaries {
		check("boundary", b.ID, b.Source)
	}
	for _, d := range m.Dependencies {
		// dependency allow  URL/to type source( formkind   peter( ) then has source): 
		// only giveoutchar  type source time   filerefer . 
		var s string
		if len(d.Source) > 0 && json.Unmarshal(d.Source, &s) == nil && strings.TrimSpace(s) != "" {
			check("dependency", d.On, s)
		}
	}
	return out
}

func TestSM1EveryClaimHasResolvableSource(t *testing.T) {
	root := scRepoRoot(t)
	path, m := pmLoad(t)
	claims := len(m.Capabilities) + len(m.Dormant) + len(m.Unresolved) + len(m.Domains) + len(m.Boundaries)
	if claims == 0 {
		t.Fatalf("[SM-1][防空过] %s 一条可核陈述都没有", path)
	}
	//  data  :      source, also  ed  source( to). 
	bad := pmModel{Capabilities: []pmCapability{{ID: "x", Kind: "tool", Source: "no/such/file.go", Status: "verified"}}}
	if len(pmUnresolvableSources(root, bad)) == 0 {
		t.Error("[SM-1 自检] 指向不存在文件的 source 没被判红 → 判据假绿")
	}
	good := pmModel{Capabilities: []pmCapability{{ID: "x", Kind: "tool", Source: "tools/registry.go", Status: "verified"}}}
	if got := pmUnresolvableSources(root, good); len(got) != 0 {
		t.Errorf("[SM-1 自检] 合法 source 被误判: %v", got)
	}
	if got := pmUnresolvableSources(root, m); len(got) != 0 {
		for _, v := range got {
			t.Errorf("%s", v)
		}
	}
	t.Logf("[SM-1] %s：核了 %d 条陈述的 source 指针", path, claims)
}

// ---------- SM-2:  toto  ----------

func TestSM2BidirectionalReconcileWithCode(t *testing.T) {
	path, m := pmLoad(t)
	truth := pmDeriveTruth(t)

	//  data  :    /    / empty type  kindnoteinall    ( torevexample + preventemptyed). 
	over := m
	over.Capabilities = append(append([]pmCapability{}, m.Capabilities...),
		pmCapability{ID: "deploy.run", Kind: "tool", Source: "tools/registry.go", Status: "verified"})
	if !pmHasViolation(pmReconcile(over, truth), "虚报") {
		t.Error("[SM-2 自检] 注入的虚报能力没被判红")
	}
	under := m
	under.Capabilities = append([]pmCapability{}, m.Capabilities[1:]...)
	if !pmHasViolation(pmReconcile(under, truth), "隐瞒") {
		t.Error("[SM-2 自检] 漏掉一个能力没被判红（隐瞒漏判）")
	}
	if len(pmReconcile(pmModel{}, truth)) == 0 {
		t.Error("[SM-2 自检] 空模型被判绿 → 防空过失败")
	}
	if got := pmReconcile(m, truth); len(got) != 0 {
		for _, v := range got {
			t.Errorf("[SM-2] %s", v)
		}
	}
	t.Logf("[SM-2] %s：与代码真值双向对账（%d 契约 cap / %d live 意图 / %d dormant / %d 域 / %d 别名）",
		path, len(truth.ToolCaps), len(truth.LiveIntents), len(truth.DormantIntents), len(truth.Domains), len(truth.AliasUnion))
}

// ---------- SM-3:  state   ----------

type pmClaim struct {
	Section string
	ID      string
	Status  string
	Detail  string // unknown time    as  
	Source  string
}

func pmClaims(m pmModel) []pmClaim {
	var out []pmClaim
	for _, c := range m.Capabilities {
		out = append(out, pmClaim{"capability", c.ID, c.Status, c.Note, c.Source})
	}
	for _, d := range m.Dormant {
		out = append(out, pmClaim{"dormant", d.ID, d.Status, d.Why, d.Source})
	}
	for _, u := range m.Unresolved {
		out = append(out, pmClaim{"unresolved", u.ID, u.Status, u.Why, u.SeenIn})
	}
	for _, d := range m.Domains {
		out = append(out, pmClaim{"domain", d.Name, d.Status, d.RiskDefault, d.Source})
	}
	for _, b := range m.Boundaries {
		out = append(out, pmClaim{"boundary", b.ID, b.Status, b.Claim, b.Source})
	}
	for _, d := range m.Dependencies {
		out = append(out, pmClaim{"dependency", d.On, d.Status, d.Note, ""})
	}
	out = append(out, pmClaim{"state", "state", m.State.Status, m.State.Source, ""})
	for _, q := range m.OpenQuestions {
		out = append(out, pmClaim{"open_question", q.Q, q.Status, q.WhyItMatters, ""})
	}
	return out
}

var pmThreeStates = map[string]bool{"verified": true, "inferred": true, "unknown": true}

// pmSourceRequiredSections:  formneedrequire  source  node(dependencies  allow  ). 
var pmSourceRequiredSections = map[string]bool{
	"capability": true, "dormant": true, "unresolved": true, "domain": true, "boundary": true,
}

func pmStatusViolations(m pmModel) []string {
	var out []string
	claims := pmClaims(m)
	if len(claims) == 0 {
		return append(out, "[防空过] 一条陈述都没有 → 三态判据会假绿")
	}
	nonVerified := 0
	for _, c := range claims {
		if !pmThreeStates[c.Status] {
			out = append(out, fmt.Sprintf("[SM-3] %s %s 的 status=%q 不在三态内（verified|inferred|unknown）",
				c.Section, c.ID, c.Status))
		} else if c.Status != "verified" {
			nonVerified++
		}
		if c.Status == "unknown" && strings.TrimSpace(c.Detail) == "" {
			out = append(out, fmt.Sprintf("[SM-3] %s %s 标了 unknown 却没说为什么", c.Section, c.ID))
		}
		if c.Status == "verified" && pmSourceRequiredSections[c.Section] && strings.TrimSpace(c.Source) == "" {
			out = append(out, fmt.Sprintf("[SM-3] %s %s 标了 verified 却没有 source", c.Section, c.ID))
		}
	}
	if nonVerified == 0 {
		out = append(out, "[SM-3] 诚实率=0：所有陈述都 verified，没有一个 inferred/unknown —— 不是模型，是宣传")
	}
	return out
}

func TestSM3ThreeStatesAndHonestUnknowns(t *testing.T) {
	path, m := pmLoad(t)

	//  data  :   statevalue    ; safety verified     (  rate). 
	badStatus := pmModel{Capabilities: []pmCapability{{ID: "x", Kind: "tool", Status: "ok", Source: "tools/registry.go"}}}
	if !pmHasViolation(pmStatusViolations(badStatus), "不在三态内") {
		t.Error("[SM-3 自检] status=ok 没被判红 → 判据假绿")
	}
	allVerified := pmModel{Capabilities: []pmCapability{{ID: "x", Kind: "tool", Status: "verified", Source: "tools/registry.go"}}}
	if !pmHasViolation(pmStatusViolations(allVerified), "诚实率") {
		t.Error("[SM-3 自检] 全 verified 没被判红（没有 unknown 等于没有模型）")
	}
	if len(pmStatusViolations(pmModel{})) == 0 {
		t.Error("[SM-3 自检] 空模型被判绿 → 防空过失败")
	}
	if got := pmStatusViolations(m); len(got) != 0 {
		for _, v := range got {
			t.Errorf("%s", v)
		}
	}
	claims := pmClaims(m)
	t.Logf("[SM-3] %s：%d 条陈述的三态已核", path, len(claims))
}

// ---------- SM-4: verified       and code   ----------

func pmGitAncestor(root, sha string) error {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", sha, "HEAD")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git merge-base --is-ancestor %s HEAD 失败: %v %s", sha, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// pmBoundaryCheckers:        boundary   (id ->    num). 
//    ed =  codechangebut type  ingchange(SM-4  ). 
func pmBoundaryCheckers(root string) map[string]func(t *testing.T) error {
	return map[string]func(t *testing.T) error{
		"no-deploy-tool": func(t *testing.T) error {
			tr, _ := loadRegistries(t)
			for _, c := range tr.All() {
				if c.Name == "deploy" {
					return errors.New("注册表里出现了 deploy 契约")
				}
			}
			return nil
		},
		"no-git-push": func(t *testing.T) error {
			tr, _ := loadRegistries(t)
			c, ok := tr.Get("git")
			if !ok {
				return errors.New("git 契约不存在")
			}
			if hasCap(c.Caps, "push") {
				return errors.New("git 契约出现了 push cap")
			}
			return nil
		},
		"run-exec-high": func(t *testing.T) error {
			tr, _ := loadRegistries(t)
			c, ok := tr.Get("run")
			if !ok {
				return errors.New("run 契约不存在")
			}
			if c.Risk["exec"] != "high" {
				return fmt.Errorf("run.exec risk=%q（期望 high）", c.Risk["exec"])
			}
			return nil
		},
		"vault-creds-readonly": func(t *testing.T) error {
			_, sr := loadRegistries(t)
			man, ok := sr.Get("vault-creds")
			if !ok {
				return errors.New("vault-creds 域不存在")
			}
			if man.Perms.Write || len(man.Perms.Exec) != 0 {
				return fmt.Errorf("vault-creds 不再是只读: write=%v exec=%v", man.Perms.Write, man.Perms.Exec)
			}
			return nil
		},
		"external-domain-unusable": func(t *testing.T) error {
			tr, sr := loadRegistries(t)
			real := map[string]bool{}
			for _, c := range tr.All() {
				real[c.Name] = true
			}
			man, ok := sr.Get("external")
			if !ok {
				return errors.New("external 域不存在")
			}
			for _, tl := range man.Tools {
				if real[tl] {
					return fmt.Errorf("external 域词表里的 %q 已变成真实契约（该陈述需要更新）", tl)
				}
			}
			return nil
		},
		"no-third-party-deps": func(t *testing.T) error {
			raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
			if err != nil {
				return err
			}
			if strings.Contains(string(raw), "require") {
				return errors.New("go.mod 出现 require（已有第三方依赖）")
			}
			return nil
		},
		"manifest-not-implemented": func(t *testing.T) error {
			tr, sr := loadRegistries(t)
			m := ExportManifest(tr, sr)
			if len(m.Tools) != 0 || len(m.Intents) != 0 || len(m.Domains) != 0 {
				return errors.New("ExportManifest 已不再是桩 → 模型必须刷新这条陈述")
			}
			return nil
		},
		"planner-not-implemented": func(t *testing.T) error {
			tr, sr := loadRegistries(t)
			_, err := LocalPlanner{}.Plan("任意目标", ExportManifest(tr, sr))
			if !errors.Is(err, ErrNotImplemented) {
				return fmt.Errorf("LocalPlanner.Plan 已不再是桩（err=%v）→ 模型必须刷新这条陈述", err)
			}
			return nil
		},
		"replan-not-implemented": func(t *testing.T) error {
			tr, sr := loadRegistries(t)
			_, err := Replan(Plan{}, StepFailure{}, ExportManifest(tr, sr), 1)
			if !errors.Is(err, ErrNotImplemented) {
				return fmt.Errorf("Replan 已不再是桩（err=%v）→ 模型必须刷新这条陈述", err)
			}
			return nil
		},
	}
}

func TestSM4VerifiedClaimsStillMatchCode(t *testing.T) {
	root := scRepoRoot(t)
	path, m := pmLoad(t)

	if strings.TrimSpace(m.Subject.Commit) == "" {
		t.Fatalf("[SM-4][防空过] subject.commit 为空 → 陈述没有真值基线")
	}
	if err := pmGitAncestor(root, m.Subject.Commit); err != nil {
		t.Errorf("[SM-4] 真值基线不可达: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, m.Subject.ProducedAt); err != nil {
		t.Errorf("[SM-4] produced_at=%q 不是 RFC3339: %v", m.Subject.ProducedAt, err)
	}
	if len(m.State.Tests) == 0 {
		t.Errorf("[SM-4] state.tests 为空 → 「我在哪」缺失")
	}

	//  data  :      ed,      be  ( to). 
	if err := pmBoundaryCheckers(root)["no-deploy-tool"](t); err != nil {
		t.Errorf("[SM-4 自检] 现成的真陈述复核不过: %v", err)
	}
	falseClaim := func(t *testing.T) error {
		tr, _ := loadRegistries(t)
		c, ok := tr.Get("git")
		if !ok {
			return errors.New("git 契约不存在")
		}
		if hasCap(c.Caps, "push") {
			return nil
		}
		return errors.New("git 没有 push cap（这是一条假陈述，必须被判红）")
	}
	if err := falseClaim(t); err == nil {
		t.Error("[SM-4 自检] 假陈述没被判红 → 判据假绿")
	}

	checkers := pmBoundaryCheckers(root)
	checked := 0
	for _, b := range m.Boundaries {
		if b.Status != "verified" {
			continue
		}
		fn, ok := checkers[b.ID]
		if !ok {
			continue
		}
		checked++
		if err := fn(t); err != nil {
			t.Errorf("[SM-4] 陈述 %q 与代码不符（代码变了，模型没跟着变）: %v", b.ID, err)
		}
	}
	if checked == 0 {
		t.Errorf("[SM-4][防空过] %s 里没有一条可机械复核的 verified 陈述 → 本判据无从判定", path)
	}
	t.Logf("[SM-4] %s：机械复核了 %d/%d 条 boundary 陈述（truth base %s）",
		path, checked, len(m.Boundaries), m.Subject.Commit[:min(7, len(m.Subject.Commit))])
}

// ---------- SM-5: occurproduce  sideand typeartifact  same   value ----------

// pmCompareManifestArtifact    plan.ExportManifest(occurproduce  )and typeartifact(eval side out ). 
func pmCompareManifestArtifact(mm Manifest, art pmModel) []string {
	var out []string
	artTools := map[string]bool{}
	artCaps := map[string]bool{}
	for _, c := range art.Capabilities {
		if c.Kind != "tool" {
			continue
		}
		artCaps[c.ID] = true
		if i := strings.Index(c.ID, "."); i > 0 {
			artTools[c.ID[:i]] = true
		}
	}
	mmTools := map[string]bool{}
	mmCaps := map[string]bool{}
	for _, c := range mm.Tools {
		mmTools[c.Name] = true
		for _, cp := range c.Caps {
			mmCaps[c.Name+"."+cp] = true
		}
	}
	for id := range mmCaps {
		if !artCaps[id] {
			out = append(out, fmt.Sprintf("[SM-5] ExportManifest 有 %q，模型产物没有（双实现漂移）", id))
		}
	}
	for id := range artCaps {
		if !mmCaps[id] {
			out = append(out, fmt.Sprintf("[SM-5] 模型产物有 %q，ExportManifest 没有（双实现漂移）", id))
		}
	}
	for n := range mmTools {
		if !artTools[n] {
			out = append(out, fmt.Sprintf("[SM-5] ExportManifest 多了工具 %q", n))
		}
	}
	for n := range artTools {
		if !mmTools[n] {
			out = append(out, fmt.Sprintf("[SM-5] ExportManifest 少了工具 %q", n))
		}
	}

	artLive := map[string]bool{}
	for _, c := range art.Capabilities {
		if c.Kind == "intent" {
			artLive[c.ID] = true
		}
	}
	mmLive := map[string]bool{}
	mmDormant := map[string]bool{}
	for _, it := range mm.Intents {
		if it.Live {
			mmLive[it.Value] = true
		}
		if it.Dormant {
			mmDormant[it.Value] = true
		}
	}
	for v := range mmLive {
		if !artLive[v] {
			out = append(out, fmt.Sprintf("[SM-5] ExportManifest 说 %s live，模型产物没有", v))
		}
	}
	for v := range artLive {
		if !mmLive[v] {
			out = append(out, fmt.Sprintf("[SM-5] 模型产物说 %s live，ExportManifest 没有", v))
		}
	}
	artDormant := map[string]bool{}
	for _, d := range art.Dormant {
		artDormant[d.ID] = true
	}
	for v := range mmDormant {
		if !artDormant[v] {
			out = append(out, fmt.Sprintf("[SM-5] ExportManifest 说 %s dormant，模型产物没有", v))
		}
	}
	for v := range artDormant {
		if !mmDormant[v] {
			out = append(out, fmt.Sprintf("[SM-5] 模型产物说 %s dormant，ExportManifest 没有", v))
		}
	}

	artDomains := map[string]bool{}
	for _, d := range art.Domains {
		artDomains[d.Name] = true
	}
	mmDomains := map[string]bool{}
	for _, d := range mm.Domains {
		mmDomains[d.Name] = true
	}
	for n := range mmDomains {
		if !artDomains[n] {
			out = append(out, fmt.Sprintf("[SM-5] ExportManifest 有域 %q，模型产物没有", n))
		}
	}
	for n := range artDomains {
		if !mmDomains[n] {
			out = append(out, fmt.Sprintf("[SM-5] 模型产物有域 %q，ExportManifest 没有", n))
		}
	}
	return out
}

func TestSM5ManifestAgreesWithModelArtifact(t *testing.T) {
	path, art := pmLoad(t)
	tr, sr := loadRegistries(t)
	mm := ExportManifest(tr, sr)

	//  data  (first   data  , again artifact): emptylist /       all    . 
	if !pmHasViolation(pmCompareManifestArtifact(Manifest{}, art), "ExportManifest 少了工具") {
		t.Error("[SM-5 自检] 空清单没被判红 → 判据假绿")
	}
	over := mm
	over.Tools = append(append([]Capability{}, mm.Tools...),
		Capability{Name: "deploy", Caps: []string{"run"}, Source: "tools/registry.go"})
	if !pmHasViolation(pmCompareManifestArtifact(over, art), "多了工具") {
		t.Error("[SM-5 自检] 注入的虚报工具没被判红")
	}

	if len(mm.Tools) == 0 {
		t.Fatalf("[SM-5] 生产能力侧清单为空（ExportManifest 是 P1 桩 → 先红）：无法与 %s 对账", path)
	}
	if got := pmCompareManifestArtifact(mm, art); len(got) != 0 {
		for _, v := range got {
			t.Errorf("%s", v)
		}
	}
	t.Logf("[SM-5] %s 与 plan.ExportManifest 一致（%d 工具 / %d 意图 / %d 域）",
		path, len(mm.Tools), len(mm.Intents), len(mm.Domains))
}
