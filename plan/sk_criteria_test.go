//go:build vhsplan

// sk_criteria_test.go -- VHS-PLAN-001 §3: SK-1..SK-6(first ). 
//
//  value  (alreadyby        out): 
//
//	   ← tools.LoadContracts(dir).All()
//	intent ← contract.Intent*    × occurproduce code use(live/dormant)
//	domain   ← space.Load(dir).List()/Get()
//
//   : go test -tags vhsplan ./plan
package plan

import (
	"path/filepath"
	"reflect"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/space"
	"voicesign-harness/tools"
)

//  value(   outclose  period ; numchar    ,  is write  ). 
var (
	wantTools = []string{"file", "git", "run", "search", "test", "verify"}
	// live/dormant  path: by DSH  decide(2026-10-03) use**occurproduce code use**rule
	// --   listdescribe"  time     ",    use(router/router_test.go:37)
	//   become  time  . thus APP_LAUNCH   dormant: 16 live / 2 dormant. 
	wantLive    = []string{"ASK", "COMMIT", "DEBUG", "DEPLOY", "EDIT", "FILE_LIST", "FILE_READ", "INFO", "NOTE", "ORCHESTRATE", "QUERY", "REGISTER_TOOL", "SHELL", "TEST", "TIME", "UNKNOWN"}
	wantDormant = []string{"APP_LAUNCH", "FILE_WRITE"}
	wantDomains = []string{"external", "global", "project", "sandbox", "vault-creds", "vault-notes"}
	// domain diffnamein   note table  store  (     11 place). 
	wantUnresolvedAliases = []string{"ask", "deploy", "file-append", "http", "note", "query", "read"}
)

func loadRegistries(t *testing.T) (*tools.Registry, *space.Registry) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	tr, err := tools.LoadContracts(missing)
	if err != nil {
		t.Fatalf("[SK] 工具注册表导出失败: %v", err)
	}
	sr, err := space.Load(missing)
	if err != nil {
		t.Fatalf("[SK] 域注册表导出失败: %v", err)
	}
	return tr, sr
}

func (m Manifest) toolSet() map[string]Capability {
	out := map[string]Capability{}
	for _, c := range m.Tools {
		out[c.Name] = c
	}
	return out
}

func (m Manifest) liveIntents() map[string]bool {
	out := map[string]bool{}
	for _, i := range m.Intents {
		if i.Live {
			out[i.Value] = true
		}
	}
	return out
}

func (m Manifest) dormantIntents() map[string]bool {
	out := map[string]bool{}
	for _, i := range m.Intents {
		if i.Dormant {
			out[i.Value] = true
		}
	}
	return out
}

// SK-1    : list     all   note table   store . 
func TestSK1NoOverclaim(t *testing.T) {
	tr, sr := loadRegistries(t)
	m := ExportManifest(tr, sr)
	real := map[string]bool{}
	for _, c := range tr.All() {
		real[c.Name] = true
	}
	if len(m.Tools) == 0 {
		t.Fatal("[SK-1] 能力清单没有工具（P1 桩，先红）")
	}
	for _, c := range m.Tools {
		if !real[c.Name] {
			t.Errorf("[SK-1] 虚报能力 %q：契约注册表里不存在", c.Name)
		}
	}
}

// SK-2    : note table has , list    . 
func TestSK2NoOmission(t *testing.T) {
	tr, sr := loadRegistries(t)
	m := ExportManifest(tr, sr)
	got := m.toolSet()
	for _, c := range tr.All() {
		if _, ok := got[c.Name]; !ok {
			t.Errorf("[SK-2] 隐瞒能力 %q：注册表里有却没进清单", c.Name)
		}
	}
	for _, name := range wantTools {
		if _, ok := got[name]; !ok {
			t.Errorf("[SK-2] 清单缺内置工具 %q", name)
		}
	}
}

// SK-3  boundaryapprove :         limitrestrict(risk / allowed_spaces /  riskneedconfirm). 
func TestSK3BoundariesAccurate(t *testing.T) {
	tr, sr := loadRegistries(t)
	m := ExportManifest(tr, sr)
	if len(m.Tools) == 0 {
		t.Fatal("[SK-3] 能力清单没有工具（P1 桩，先红）——空清单不得让本判据空过")
	}
	byName := map[string]contract.ToolContract{}
	for _, c := range tr.All() {
		byName[c.Name] = c
	}
	for _, cap := range m.Tools {
		c, ok := byName[cap.Name]
		if !ok {
			continue // SK-1 already 
		}
		if !reflect.DeepEqual(cap.Risk, c.Risk) {
			t.Errorf("[SK-3] %s 的 risk 与契约不符: %v vs %v", cap.Name, cap.Risk, c.Risk)
		}
		if !reflect.DeepEqual(cap.AllowedSpaces, c.AllowedSpaces) {
			t.Errorf("[SK-3] %s 的 allowed_spaces 与契约不符", cap.Name)
		}
		for capName, risk := range c.Risk {
			if (risk == "high" || risk == "irreversible") && !cap.NeedsConfirm[capName] {
				t.Errorf("[SK-3] %s.%s 风险=%s 却未标记需人工确认（把'需确认'说成'我能做'）", cap.Name, capName, risk)
			}
		}
	}
}

// SK-4       +    code: notein become  afterlist   ingchange. 
func TestSK4SourceAuditableAndDerived(t *testing.T) {
	tr, sr := loadRegistries(t)
	m1 := ExportManifest(tr, sr)
	m2 := ExportManifest(tr, sr)
	if !reflect.DeepEqual(m1, m2) {
		t.Error("[SK-4] 两次导出不一致（不可回放）")
	}
	if len(m1.Tools) == 0 {
		t.Fatal("[SK-4] 能力清单为空（P1 桩，先红）")
	}
	for _, c := range m1.Tools {
		if c.Source == "" {
			t.Errorf("[SK-4] 工具 %q 缺 source（SK-2 对账需要它）", c.Name)
		}
	}
	for _, i := range m1.Intents {
		if i.Source == "" {
			t.Errorf("[SK-4] 意图 %q 缺 source", i.Value)
		}
	}
	for _, d := range m1.Domains {
		if d.Source == "" {
			t.Errorf("[SK-4] 域 %q 缺 source", d.Name)
		}
	}

	// close ity  :  is  code -- notein   become  , list  rev  . 
	synth := &tools.Registry{Contracts: map[string]contract.ToolContract{}}
	for _, c := range tr.All() {
		synth.Contracts[c.Name] = c
	}
	synth.Contracts["demo-probe"] = contract.ToolContract{
		Name: "demo-probe", Version: "1.0", Source: "test",
		Caps: []string{"ping"}, Params: map[string]string{},
		Risk: map[string]string{"ping": "none"}, AllowedSpaces: []string{"sandbox"},
	}
	m3 := ExportManifest(synth, sr)
	if _, ok := m3.toolSet()["demo-probe"]; !ok {
		t.Error("[SK-4] 注入的合成契约没有出现在清单 → 清单是硬编码的，不是从代码导出的")
	}
}

// SK-5 intent: live     etcat   out  value; dormant   cur     . 
func TestSK5IntentsLiveAndDormant(t *testing.T) {
	tr, sr := loadRegistries(t)
	m := ExportManifest(tr, sr)
	live, dormant := m.liveIntents(), m.dormantIntents()
	if len(live) == 0 {
		t.Fatal("[SK-5] 清单没有任何 live 意图（P1 桩，先红）")
	}
	want := map[string]bool{}
	for _, v := range wantLive {
		want[v] = true
	}
	if !reflect.DeepEqual(live, want) {
		t.Errorf("[SK-5] live 意图集合与真值不符：got=%v want=%v", live, want)
	}
	for _, v := range wantDormant {
		if live[v] {
			t.Errorf("[SK-5] %s 是 dormant（无生产引用），不得当成 live 能力", v)
		}
		if !dormant[v] {
			t.Errorf("[SK-5] %s 是 dormant，清单未登记（不隐瞒也不虚报）", v)
		}
	}
}

// SK-6 domainanddiffname  : domain    out; domain diffname        listtable. 
func TestSK6DomainsAndAliasHonesty(t *testing.T) {
	tr, sr := loadRegistries(t)
	m := ExportManifest(tr, sr)
	var gotDomains []string
	for _, d := range m.Domains {
		gotDomains = append(gotDomains, d.Name)
	}
	if !reflect.DeepEqual(sortedCopy(gotDomains), wantDomains) {
		t.Errorf("[SK-6] 域集合与 space.List() 不符：got=%v want=%v", gotDomains, wantDomains)
	}

	// diffname  :   note table  store  diffname,    outnow     listtable ( then SK-1   ). 
	real := map[string]bool{}
	for _, c := range tr.All() {
		real[c.Name] = true
	}
	aliases := map[string]bool{}
	for _, d := range m.Domains {
		for _, a := range d.Aliases {
			aliases[a] = true
		}
	}
	for _, a := range wantUnresolvedAliases {
		if !aliases[a] {
			t.Errorf("[SK-6] 别名 %q 被丢弃了（丢弃别名 = 隐瞒：管线确实在用）", a)
		}
		if _, bad := m.toolSet()[a]; bad && !real[a] {
			t.Errorf("[SK-6] 别名 %q 混进了能力列表（别名混入 = 虚报：它不是可调用能力）", a)
		}
	}
	for _, c := range m.Tools {
		if aliases[c.Name] && !real[c.Name] {
			t.Errorf("[SK-6] 别名 %q 出现在 Tools 里，但契约注册表没有它", c.Name)
		}
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
