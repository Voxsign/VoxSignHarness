package refer

import (
	"strings"
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/memory"
)

func TestNewNilDictOK(t *testing.T) {
	r := New(nil)
	if r == nil {
		t.Fatal("New(nil) 应返回非 nil")
	}
}

// 用例 4：「它」指上一条回执文件 → 上下文命中，不再回问。
func TestResolve_AnaphoraToRecentFile(t *testing.T) {
	r := New(nil)
	r.Recent = []RecentEntity{
		{Space: "voicesign-harness", Entity: "space/space.go", Kind: "file", Ts: "2026-10-02T10:00:00"},
	}
	it := &contract.Intent{
		Intent:        contract.IntentEdit,
		CorrectedText: "把它再改一下",
	}
	got, err := r.Resolve(it, "voicesign-harness")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Ask != "" {
		t.Fatalf("用例4: 不应回问, Ask=%q", got.Ask)
	}
	if got.Target == nil || got.Target.Entity != "space/space.go" {
		t.Fatalf("用例4: Target=%+v 期望 space/space.go", got.Target)
	}
	if got.Target.RefType != "anaphora" {
		t.Fatalf("RefType=%q 期望 anaphora", got.Target.RefType)
	}
}

// 用例 3：跨域歧义指代 → 显式回问"哪个域？"，不猜。
func TestResolve_CrossDomainAmbiguous(t *testing.T) {
	r := New(nil)
	r.Recent = []RecentEntity{
		{Space: "voxbuybot", Entity: "src/Checkout.js", Kind: "file", Ts: "2026-10-02T09:00:00"},
		{Space: "vault-notes", Entity: "idea-12.md", Kind: "file", Ts: "2026-10-02T11:00:00"},
	}
	it := &contract.Intent{Intent: contract.IntentEdit, CorrectedText: "改一下它"}
	got, _ := r.Resolve(it, "voicesign-harness") // 当前域两个候选都不命中
	if got.Ask == "" || !strings.Contains(got.Ask, "哪个域") {
		t.Fatalf("用例3: 应回问「哪个域？」, Ask=%q", got.Ask)
	}
	if got.Target != nil && got.Target.Entity != "" {
		t.Fatalf("用例3: 跨域歧义不许猜, Target=%+v", got.Target)
	}
}

// 当前域有最近文件 → 即便别处也有，优先当前域，不回问。
func TestResolve_PrefersCurrentSpace(t *testing.T) {
	r := New(nil)
	r.Recent = []RecentEntity{
		{Space: "voxbuybot", Entity: "other.js", Kind: "file", Ts: "2026-10-02T12:00:00"},
		{Space: "voicesign-harness", Entity: "space.go", Kind: "file", Ts: "2026-10-02T08:00:00"},
	}
	it := &contract.Intent{Intent: contract.IntentEdit, CorrectedText: "改一下它"}
	got, _ := r.Resolve(it, "voicesign-harness")
	if got.Ask != "" || got.Target == nil || got.Target.Entity != "space.go" {
		t.Fatalf("应优先当前域候选: Ask=%q Target=%+v", got.Ask, got.Target)
	}
}

// 词典层 100% 命中 → 直接消解，不查上下文。
func TestResolve_DictLayer(t *testing.T) {
	dict := &memory.Dictionary{Terms: []memory.Term{
		{Term: "Mansour", Variants: []string{"美墅"}, Category: "人名"},
	}}
	r := New(dict)
	it := &contract.Intent{Intent: contract.IntentNote, CorrectedText: "记一下美墅的报价"}
	got, _ := r.Resolve(it, "voicesign-harness")
	if got.Target == nil || got.Target.Entity != "Mansour" {
		t.Fatalf("词典层应命中 Mansour: %+v", got.Target)
	}
	if got.Target.RefType != "dict" {
		t.Fatalf("RefType=%q 期望 dict", got.Target.RefType)
	}
}

// 无指代词 → 原样返回，不动 Target。
func TestResolve_NoAnaphora(t *testing.T) {
	r := New(nil)
	r.Recent = []RecentEntity{{Space: "x", Entity: "a.go", Kind: "file", Ts: "t1"}}
	it := &contract.Intent{Intent: contract.IntentQuery, CorrectedText: "查一下今天的测试结果"}
	got, _ := r.Resolve(it, "x")
	if got.Target != nil {
		t.Fatalf("无指代词不应填 Target: %+v", got.Target)
	}
}

// 规则层无候选 + ModelFn 高置信 → 语言层消解。
func TestResolve_ModelLayer(t *testing.T) {
	r := New(nil)
	r.ModelFn = func(q string, cands []string) (string, float64) {
		return "报价模板.docx", 0.9
	}
	it := &contract.Intent{Intent: contract.IntentEdit, CorrectedText: "把上次那个改一下"}
	got, _ := r.Resolve(it, "voxbuybot")
	if got.Target == nil || got.Target.Entity != "报价模板.docx" {
		t.Fatalf("语言层应消解: %+v", got.Target)
	}
}

// 无候选 + ModelFn nil + 有指代 → 必须 Ask（不许静默吃掉）。
func TestResolve_LowConfAsks(t *testing.T) {
	r := New(nil) // 无 Recent、无 ModelFn
	it := &contract.Intent{Intent: contract.IntentEdit, CorrectedText: "把它改一下"}
	got, _ := r.Resolve(it, "voicesign-harness")
	if got.Ask == "" {
		t.Fatal("低置信必须显式回问，不许静默")
	}
}

// 已显式目标 → 词典规范化后直接返回。
func TestResolve_ExplicitTargetUntouched(t *testing.T) {
	dict := &memory.Dictionary{Terms: []memory.Term{
		{Term: "VoxSign", Variants: []string{"voxsign"}},
	}}
	r := New(dict)
	it := &contract.Intent{
		Intent:        contract.IntentEdit,
		CorrectedText: "改 voxsign",
		Target:        &contract.Target{Entity: "voxsign", RefType: "explicit"},
	}
	got, _ := r.Resolve(it, "voicesign-harness")
	if got.Target.Entity != "VoxSign" {
		t.Fatalf("显式目标应被词典规范化为 VoxSign, got %q", got.Target.Entity)
	}
}

// Solidify 把确认别名写进词典（内存词典 Path 空 → 仅追加内存）。
func TestSolidify(t *testing.T) {
	dict := &memory.Dictionary{}
	r := New(dict)
	if err := r.Solidify("阿卜杜拉", "阿卜杜拉赫曼"); err != nil {
		t.Fatalf("Solidify: %v", err)
	}
	if len(dict.Terms) != 1 || dict.Terms[0].Term != "阿卜杜拉" {
		t.Fatalf("词典未追加: %+v", dict.Terms)
	}
}

// ---- M4 选项按钮化：歧义回问产出结构化候选 ----

// 歧义输入（"帮我看看那个文件"）+ 跨域多个最近文件 → 返回 2-4 个 id/label 候选且不重复。
func TestResolveOptionsStructured(t *testing.T) {
	r := New(nil)
	r.Recent = []RecentEntity{
		{Space: "voxbuybot", Entity: "src/Checkout.js", Kind: "file", Ts: "2026-10-02T09:00:00"},
		{Space: "vault-notes", Entity: "idea-12.md", Kind: "file", Ts: "2026-10-02T11:00:00"},
		{Space: "voicesign-harness", Entity: "space/space.go", Kind: "file", Ts: "2026-10-02T10:00:00"},
	}
	it := &contract.Intent{Intent: contract.IntentQuery, CorrectedText: "帮我看看那个文件"}
	// 当前 spaceID 不在任何最近候选的域里 → 跨域歧义 → 回问 + 结构化候选
	got, opts, err := r.ResolveOptions(it, "medsupply")
	if err != nil {
		t.Fatalf("ResolveOptions: %v", err)
	}
	if got.Ask == "" {
		t.Fatalf("跨域歧义应回问, Ask=%q", got.Ask)
	}
	if len(opts) < 2 || len(opts) > 4 {
		t.Fatalf("候选数应在 2-4, got %d: %+v", len(opts), opts)
	}
	seen := map[string]bool{}
	for _, o := range opts {
		if o.ID == "" || o.Label == "" {
			t.Fatalf("候选缺 id/label: %+v", o)
		}
		if seen[o.ID] {
			t.Fatalf("候选 id 重复: %s", o.ID)
		}
		seen[o.ID] = true
		if !strings.HasPrefix(o.ID, "rec:") && !strings.HasPrefix(o.ID, "dict:") {
			t.Fatalf("候选 id 前缀不合规: %s", o.ID)
		}
	}
}

// 无候选场景（无 Recent、无词典命中、有指代词）→ Options 为空，仅 Ask 文本。
func TestResolveOptionsEmptyWhenNoCandidates(t *testing.T) {
	r := New(nil)
	it := &contract.Intent{Intent: contract.IntentEdit, CorrectedText: "把它改一下"}
	got, opts, err := r.ResolveOptions(it, "voicesign-harness")
	if err != nil {
		t.Fatalf("ResolveOptions: %v", err)
	}
	if got.Ask == "" {
		t.Fatal("无候选低置信应回问")
	}
	if len(opts) != 0 {
		t.Fatalf("无候选时 Options 应为空, got %+v", opts)
	}
}

// Resolve（冻结签名）仍可用且不破坏分层。
func TestResolveBackwardsCompatible(t *testing.T) {
	r := New(nil)
	r.Recent = []RecentEntity{{Space: "x", Entity: "a.go", Kind: "file", Ts: "t1"}}
	got, err := r.Resolve(&contract.Intent{Intent: contract.IntentEdit, CorrectedText: "改它"}, "x")
	if err != nil || got.Target == nil || got.Target.Entity != "a.go" {
		t.Fatalf("Resolve 应仍消解到 a.go: %+v err=%v", got, err)
	}
}
