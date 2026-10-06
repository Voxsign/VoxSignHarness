//go:build vhsplan

// sc_sw_criteria_test.go -- VHS-SELFPROJ-001 §2②③: SC(  connectunder   control)and SW(   to     ). 
//
// and has data split ( heavy  ): 
//
//	PL-1..PL-5 / RV-1..RV-4(pl_criteria_test.go / rv_criteria_test.go):   base  close and rule
//	SK-1..SK-6(sk_criteria_test.go):   listand code value  toto 
//	basefile: ②   boundarykindbase(intent!=  ,  split     body  ,    keep ize)
//	        ③    ( pt    , classdiffneedto,      , also   allrefer , unseen     table)
//
// be  : plan.LocalPlanner(P1   -> first ).  dataread-only Plan   opencharseg,  dependency type type
// (ASR-MODEL-01), because  typealso  also first first . 
//
// owner   (SW-1,     )--  "   " obj  referto   bodyto : 
//
//	 : …(oroutnow owner / humanconfirm / human decide)      --    / produce get (SW-2   class)
//	 close: …(or aiops.example.com)               --   (    ->    close needrequire, ASR-EXT-002/004)
//	 type  : …(default / diagnose / learn)       --  disconnect(ASR-MODEL-02)
//	noout dependency                                    --   noto  refer( formwriteout ,  allow  )
//
// forbidstop:    / TBD(    ). write"needneedconfirm"but write   =  answer"   ". 
//
// lang : testdata/selfproj/plan_samples.json(    provenance;      first, constructed   tgtnote). 
//   : go test -tags vhsplan ./plan -run 'TestSC|TestSW' -v
package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// ---------- lang    ----------

type scExpect struct {
	Refused     *bool    `json:"refused"`
	StepsMin    *int     `json:"steps_min"`
	StepsMax    *int     `json:"steps_max"`
	MissingMin  *int     `json:"missing_min"`
	Owner       string   `json:"owner"`
	OwnerAny    bool     `json:"owner_any"`
	OwnerWhere  string   `json:"owner_where"` // "" =   missing  ; "anywhere" = missing/reason/step.why   place
	ForbidTools []string `json:"forbid_tools"`
	ForbidCaps  []string `json:"forbid_caps"`
}

type scSample struct {
	ID         string   `json:"id"`
	Goal       string   `json:"goal"`
	Class      string   `json:"class"`
	Provenance string   `json:"provenance"`
	Unseen     bool     `json:"unseen"`
	Assert     bool     `json:"assert"`
	Why        string   `json:"why"`
	Expect     scExpect `json:"expect"`
}

type scCorpus struct {
	Schema  string     `json:"schema"`
	Samples []scSample `json:"samples"`
}

func scRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("取工作目录失败: %v", err)
	}
	for {
		if _, serr := os.Stat(filepath.Join(dir, "go.mod")); serr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("[防空过] 从 %s 向上找不到仓库根（go.mod）", dir)
		}
		dir = parent
	}
}

// scLoadCorpus   kindbaselang , and  "lang be empty / bemodify "--emptylang  =  data  . 
func scLoadCorpus(t *testing.T) scCorpus {
	t.Helper()
	path := filepath.Join(scRepoRoot(t), "testdata", "selfproj", "plan_samples.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[防空过] 语料读不到 %s: %v", path, err)
	}
	var c scCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("[防空过] 语料解析失败 %s: %v", path, err)
	}
	if len(c.Samples) == 0 {
		t.Fatalf("[防空过] 语料为空 → 判据会在空输入上假绿: %s", path)
	}
	counts := map[string]int{}
	for _, s := range c.Samples {
		if !s.Assert {
			continue
		}
		if strings.TrimSpace(s.Why) == "" && strings.TrimSpace(s.Provenance) == "" {
			t.Fatalf("[防空过] 样本 %s 断言了却没有 provenance/why（来源不可审计）", s.ID)
		}
		counts[s.Class]++
	}
	for _, need := range []struct {
		class string
		min   int
	}{
		{"reachable", 1}, {"self_service", 1}, {"capability_gap", 2}, {"authorization_gap", 1}, {"partial", 1},
	} {
		if counts[need.class] < need.min {
			t.Fatalf("[防空过] 语料被改弱：class=%s 的断言样本 %d < %d", need.class, counts[need.class], need.min)
		}
	}
	return c
}

// scSamples getoutrefer classdiff disconnectlangkindbase; get tothen stop( allowemptyed). 
func scSamples(t *testing.T, c scCorpus, classes ...string) []scSample {
	t.Helper()
	want := map[string]bool{}
	for _, cl := range classes {
		want[cl] = true
	}
	var out []scSample
	for _, s := range c.Samples {
		if s.Assert && want[s.Class] {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatalf("[防空过] 没有 %v 类的断言样本", classes)
	}
	return out
}

// ---------- owner   (SW      state) ----------

var scOwnerPatterns = []struct {
	Name string
	Re   *regexp.Regexp
}{
	{"人", regexp.MustCompile(`(?i)owner|人[:：]|人工确认|人工裁决`)},
	{"网关", regexp.MustCompile(`网关|aiops\.example\.com`)},
	{"模型通道", regexp.MustCompile(`模型通道|\b(?:default|diagnose|learn)\b`)},
	{"无", regexp.MustCompile(`无外部依赖`)},
}

// scHumanRouted   owner   change : SW-4 need split"refer "and" noneed  ". 
var scHumanRouted = regexp.MustCompile(`(?i)owner|人[:：]|人工确认|人工裁决|找 ?人`)

// scForbiddenWords: write  wordetcat answer"   ". 
var scForbiddenWords = []string{"待定", "TBD", "tbd", "待确认"}

func scOwners(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range scOwnerPatterns {
		if p.Re.MatchString(s) {
			out[p.Name] = true
		}
	}
	return out
}

// ---------- be   ----------

func scFixture(t *testing.T) (LocalPlanner, Manifest) {
	t.Helper()
	tr, sr := loadRegistries(t)
	return LocalPlanner{}, ExportManifest(tr, sr)
}

// scPlan call  rule ; pipe"P1  "and"   "splitopen ,       dataclose . 
func scPlan(t *testing.T, tag, id, goal string, m Manifest) (Plan, bool) {
	t.Helper()
	got, err := LocalPlanner{}.Plan(goal, m)
	if err == nil {
		return got, true
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Errorf("[%s] %s：规划器未实现（P1 桩，先红）: %v", tag, id, err)
		return Plan{}, false
	}
	t.Errorf("[%s] %s：规划失败: %v", tag, id, err)
	return Plan{}, false
}

// scAliasNames    outdomain diffname(space wordtable  is     nameword)--   is  . 
func scAliasNames(t *testing.T) map[string]bool {
	t.Helper()
	tr, sr := loadRegistries(t)
	real := map[string]bool{}
	for _, c := range tr.All() {
		real[c.Name] = true
	}
	alias := map[string]bool{}
	for _, name := range sr.List() {
		man, ok := sr.Get(name)
		if !ok {
			continue
		}
		for _, tl := range man.Tools {
			if !real[tl] {
				alias[tl] = true
			}
		}
	}
	if len(alias) == 0 {
		t.Fatal("[防空过] 别名集合为空 → 本判据无从判定")
	}
	return alias
}

// scStepViolations is SC-1/SC-3     state: recv "        in" safety   . 
// writebecome  numisas to database   torevexample  (notein        ). 
func scStepViolations(id string, got Plan, m Manifest, alias map[string]bool) []string {
	var out []string
	if len(got.Steps) == 0 {
		return append(out, fmt.Sprintf("[防空过] %s 计划为空 → 空计划不得算过", id))
	}
	realTools := m.toolSet()
	intentVals := map[string]bool{}
	for _, it := range m.Intents {
		intentVals[it.Value] = true
	}
	for i, st := range got.Steps {
		if st.Tool == "" {
			out = append(out, fmt.Sprintf("%s 第 %d 步没有工具（不可执行）", id, i))
			continue
		}
		cap, ok := realTools[st.Tool]
		if !ok {
			switch {
			case alias[st.Tool]:
				out = append(out, fmt.Sprintf("%s 第 %d 步用了域级别名 %q（别名不是可调用能力）", id, i, st.Tool))
			case intentVals[st.Tool]:
				out = append(out, fmt.Sprintf("%s 第 %d 步把意图 %q 当工具用（意图 ≠ 能力）", id, i, st.Tool))
			default:
				out = append(out, fmt.Sprintf("%s 第 %d 步用了清单外能力 %q（幻觉）", id, i, st.Tool))
			}
			continue
		}
		for _, cp := range st.Caps {
			if !hasCap(cap.Caps, cp) {
				out = append(out, fmt.Sprintf("%s 第 %d 步 %s 没有 cap %q", id, i, st.Tool, cp))
			}
		}
	}
	return out
}

// scCheckStepsReal pipeonface    to  on. 
func scCheckStepsReal(t *testing.T, tag, id string, got Plan, m Manifest) {
	t.Helper()
	for _, v := range scStepViolations(id, got, m, scAliasNames(t)) {
		t.Errorf("[%s] %s", tag, v)
	}
}

// ---------- SC:   connectunder   control ----------

// SC-1   onlyuse    :    all  tolistin  , and cap  at     . 
func TestSC1StepsUseOnlyRealCapabilities(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)

	//  data  (first   data  , again artifact):   /diffname/intentcur       ,        ed. 
	fakeM := Manifest{Tools: []Capability{{Name: "file", Caps: []string{"read"}, Source: "tools/registry.go"}}}
	hallucinated := Plan{Steps: []Step{{Tool: "deploy", Action: "部署", Output: "上线"}}}
	if len(scStepViolations("自检", hallucinated, fakeM, map[string]bool{"deploy": true})) == 0 {
		t.Error("[SC-1 自检] 幻觉/别名步骤没被判红 → 判据假绿")
	}
	if len(scStepViolations("自检", Plan{Steps: []Step{}}, fakeM, map[string]bool{})) == 0 {
		t.Error("[SC-1 自检] 空计划没被判红 → 防空过失败")
	}
	legit := Plan{Steps: []Step{{Tool: "file", Caps: []string{"read"}, Action: "读取", Output: "内容"}}}
	if got := scStepViolations("自检", legit, fakeM, map[string]bool{}); len(got) != 0 {
		t.Errorf("[SC-1 自检] 合法步骤被误判: %v", got)
	}

	if len(m.Tools) == 0 {
		t.Fatalf("[SC-1][防空过] 能力清单为空（ExportManifest 是桩）→ 判据无从判定，先红")
	}
	for _, s := range scSamples(t, c, "reachable", "self_service", "partial") {
		got, ok := scPlan(t, "SC-1", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		if got.Refused {
			continue // rejectpathby SC-2/SC-3  
		}
		scCheckStepsReal(t, "SC-1", s.ID, got, m)
	}
}

// SC-1b "  " base obj   : no deploy     , domainwordtable   deploy onlyisdiffname. 
//  torevexample: list outnow deploy =   ;    outnow deploy    =   . 
func TestSC1bDeployIsNeitherToolNorStep(t *testing.T) {
	tr, _ := loadRegistries(t)
	for _, ct := range tr.All() {
		if ct.Name == "deploy" || ct.Name == "http" {
			t.Fatalf("[SC-1b] 真值侧与预期不符：%q 竟成了工具契约", ct.Name)
		}
	}
	alias := scAliasNames(t)
	for _, want := range []string{"deploy", "http", "note", "file-append", "ask", "query", "read"} {
		if !alias[want] {
			t.Errorf("[SC-1b] 域级别名 %q 不在机械导出的别名集里（真值口径可能已变）", want)
		}
	}
	c := scLoadCorpus(t)
	_, m := scFixture(t)
	for _, name := range []string{"deploy", "http"} {
		for _, ct := range m.Tools {
			if ct.Name == name {
				t.Errorf("[SC-1b] 能力清单虚报 %q：它不是工具契约（SK-1 同类）", name)
			}
		}
	}
	for _, s := range scSamples(t, c, "capability_gap", "partial") {
		got, ok := scPlan(t, "SC-1b", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		for i, st := range got.Steps {
			if alias[st.Tool] {
				t.Errorf("[SC-1b] %s 第 %d 步执行了别名 %q —— 部署类目标在本项目不可达，不得编出可执行步骤", s.ID, i, st.Tool)
			}
		}
	}
}

// SC-2   tothen   to(rev  , ②  heavyneed   ). 
func TestSC2UnreachableRefusedNotInvented(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)
	for _, s := range scSamples(t, c, "capability_gap") {
		got, ok := scPlan(t, "SC-2", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		if s.Expect.Refused != nil && *s.Expect.Refused && !got.Refused {
			t.Errorf("[SC-2] %s 目标不可达（%s）却未拒绝 → 反幻觉判据失败", s.ID, s.Provenance)
		}
		if len(got.Steps) != 0 {
			t.Errorf("[SC-2] %s 拒绝却仍给出步骤（编了一个执行不了的计划）: %+v", s.ID, got.Steps)
		}
		if len(got.Missing) < 1 {
			t.Errorf("[SC-2] %s 拒绝了却没说缺什么（PL-2 要求说明缺什么）", s.ID)
		}
	}
}

// SC-3  split     body  :      splitneed  bodyreject, need be form nowas  . 
func TestSC3PartialPlanNotWhollyExecuted(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)
	alias := scAliasNames(t)
	for _, s := range scSamples(t, c, "partial") {
		got, ok := scPlan(t, "SC-3", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		for i, st := range got.Steps {
			if alias[st.Tool] {
				t.Errorf("[SC-3] %s 第 %d 步落进不可达部分（别名 %q）：不得整体照做", s.ID, i, st.Tool)
			}
		}
		if got.Refused {
			if len(got.Steps) != 0 {
				t.Errorf("[SC-3] %s 拒绝了却仍给出步骤: %+v", s.ID, got.Steps)
			}
			continue
		}
		//  split  is allow , but      split  be out  --      = useuserbyas   all . 
		if len(got.Missing) == 0 {
			t.Errorf("[SC-3] %s 既没拒绝也没列出缺什么 → 静默吞掉了不可达的那一步", s.ID)
		}
		scCheckStepsReal(t, "SC-3", s.ID, got, m)
	}
}

// SC-4    continue before :      bekeep izeandorigkindreadback(SC-3"interruptafter frominterruptptcontinuecontinue"  need  ). 
func TestSC4PlanPersistableForResume(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)
	for _, s := range scSamples(t, c, "reachable") {
		got, ok := scPlan(t, "SC-4", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		if len(got.Steps) == 0 {
			t.Errorf("[SC-4][防空过] %s 计划为空：空计划无法证明可续，算过即假绿", s.ID)
			continue
		}
		raw, err := json.Marshal(got)
		if err != nil {
			t.Errorf("[SC-4] %s 计划无法序列化: %v", s.ID, err)
			continue
		}
		var back Plan
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Errorf("[SC-4] %s 计划无法读回: %v", s.ID, err)
			continue
		}
		if !reflect.DeepEqual(got, back) {
			t.Errorf("[SC-4] %s 计划 JSON 往返不相等（中断后无法从中断点续跑）", s.ID)
		}
		if back.Goal != s.Goal {
			t.Errorf("[SC-4] %s 往返后 goal 变了: %q → %q", s.ID, s.Goal, back.Goal)
		}
	}
}

// ---------- SW:    to      ----------

// SW-1  pt    :  is out  "   ",    allneedreferto bodyto , and  write"  ". 
// store ity(has haspipe pt out )by SC-2/SC-3  ; base data " out ofafterrefer referto ". 
func TestSW1EveryMissingEntryNamesAnOwner(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)

	//  data  ( to): empty     out owner; rule write     out. 
	for _, bad := range []string{"需要确认", "待定", "不知道找谁", ""} {
		if len(scOwners(bad)) != 0 {
			t.Errorf("[SW-1 自检] 未指对象的写法 %q 被判成有 owner → 判据假绿", bad)
		}
	}
	for _, good := range []string{"人：需要 owner 授权", "网关：缺 deploy 能力，走单一网关提需求", "模型通道：请 default 通道判定", "无外部依赖：只能放弃"} {
		if len(scOwners(good)) == 0 {
			t.Errorf("[SW-1 自检] 规范写法 %q 没被判出 owner → 判据过严", good)
		}
	}

	entries := 0
	for _, s := range scSamples(t, c, "capability_gap", "authorization_gap", "partial") {
		got, ok := scPlan(t, "SW-1", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		for _, mi := range got.Missing {
			entries++
			if len(scOwners(mi)) == 0 {
				t.Errorf("[SW-1] %s 的卡点 %q 没指到具体对象（约定：人：/ 网关：/ 模型通道：/ 无外部依赖）", s.ID, mi)
			}
			for _, w := range scForbiddenWords {
				if strings.Contains(mi, w) {
					t.Errorf("[SW-1] %s 的卡点 %q 含禁用词 %q（写『待定』等于没回答该找谁）", s.ID, mi, w)
				}
			}
		}
		for _, w := range scForbiddenWords {
			if strings.Contains(got.Reason, w) {
				t.Errorf("[SW-1] %s 的 reason %q 含禁用词 %q", s.ID, got.Reason, w)
			}
		}
	}
	if entries == 0 {
		t.Fatalf("[SW-1][防空过] 所有样本都没有任何「缺什么」条目 → 判据在空输入上假绿（先看 SC-2/SC-3 的红）")
	}
	t.Logf("[SW-1] 核了 %d 条「缺什么」的 owner 指向", entries)
}

// SW-2 splittoclassdiff:   class ->  close;   class ->  . pipe"need  " become"need  "  , reved also  . 
func TestSW2OwnerClassMatchesGapClass(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)
	n := 0
	for _, s := range scSamples(t, c, "capability_gap", "authorization_gap") {
		if s.Expect.Owner == "" {
			continue
		}
		n++
		got, ok := scPlan(t, "SW-2", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		blob := scRoutingText(got)
		if !scOwners(blob)[s.Expect.Owner] {
			t.Errorf("[SW-2] %s（class=%s）缺的是「%s」，但计划指到了 %v：%q",
				s.ID, s.Class, s.Expect.Owner, scOwnerNames(blob), blob)
		}
	}
	if n < 2 {
		t.Fatalf("[防空过] 带类别断言的样本只有 %d 条 → 分对类别无从判定", n)
	}
	//   disconnectlang kindbase  write as  (     ed). 
	for _, s := range c.Samples {
		if !s.Assert && strings.TrimSpace(s.Why) == "" {
			t.Errorf("[SW-2] 未设断言的样本 %s 没写 why（静默跳过 = 把不知道说成没意见）", s.ID)
		}
		if !s.Assert {
			t.Logf("[SW-2] 未断言（C 类候选）: %s —— %s", s.ID, s.Why)
		}
	}
}

// scRoutingText         has"   " table face:     /  by /    data. 
//  statein : rejectform(Missing refer )andrule form(Step.Why tgtnote humanconfirm)all referto . 
func scRoutingText(got Plan) string {
	parts := append([]string{}, got.Missing...)
	parts = append(parts, got.Reason)
	for _, st := range got.Steps {
		parts = append(parts, st.Why)
	}
	return strings.Join(parts, " | ")
}

func scOwnerNames(s string) []string {
	var out []string
	for name := range scOwners(s) {
		out = append(out, name)
	}
	if len(out) == 0 {
		return []string{"（没有指到任何人/网关/通道）"}
	}
	return out
}

// SW-3      :  out limit   refer ,   pipe reversible  curbecome        . 
//
//  statein ( kind rule  all ed, only "novoice  "): 
//
//	①  bodyreject + Missing refer ; 
//	②   rule , but  Reason / Step.Why   formtgtout    humanconfirm. 
func TestSW3NoSelfAuthorizationOnProtectedAction(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)
	for _, s := range scSamples(t, c, "authorization_gap") {
		got, ok := scPlan(t, "SW-3", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		routedToHuman := scOwners(scRoutingText(got))["人"]
		protected := false
		for _, st := range got.Steps {
			for _, cp := range s.Expect.ForbidCaps {
				if hasCap(st.Caps, cp) {
					protected = true
				}
			}
		}
		if !routedToHuman {
			t.Errorf("[SW-3] %s 明确要求「不用我确认」（被保护动作 %v）：计划里没有任何人工确认的指向 —— "+
				"要么拒绝并把卡点指到人，要么照常规划但写明该步须人工确认；不许无声执行",
				s.ID, s.Expect.ForbidCaps)
		}
		if protected && !routedToHuman {
			t.Errorf("[SW-3] %s 第 %v 步执行了被保护动作却没有人工路由（绕过去自己做）", s.ID, s.Expect.ForbidCaps)
		}
		if s.Expect.Refused != nil && *s.Expect.Refused && !got.Refused {
			t.Errorf("[SW-3] %s 语料要求拒绝，实际未拒绝", s.ID)
		}
	}
}

// SW-4    allrefer :     /    (ASK-0 A  )  on give , also  bereject. 
func TestSW4SelfServiceableGoalsAreNotEscalated(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)

	//  data  ( to): " noneed  "   becomeon , "referto "   becomeon . 
	if scHumanRouted.MatchString("无需人工，自己查代码即可") {
		t.Error("[SW-4 自检] 『无需人工』被误判成上抛 → 判据过严")
	}
	if !scHumanRouted.MatchString("人：需要 owner 授权") || !scHumanRouted.MatchString("请 owner 确认取舍") {
		t.Error("[SW-4 自检] 指向人的写法没被判成上抛 → 判据假绿")
	}

	for _, s := range scSamples(t, c, "self_service", "reachable") {
		got, ok := scPlan(t, "SW-4", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		if got.Refused {
			t.Errorf("[SW-4] %s 是能自己查/自己跑的（%s），却被拒绝", s.ID, s.Provenance)
		}
		if len(got.Steps) == 0 {
			t.Errorf("[SW-4][防空过] %s 计划为空 → 无法证明「没上抛」，算过即假绿", s.ID)
			continue
		}
		blob := strings.Join(got.Missing, " | ") + " || " + got.Reason
		if scHumanRouted.MatchString(blob) {
			t.Errorf("[SW-4] %s 自己能做却指向人（不该找人的不许找人）: %q", s.ID, blob)
		}
	}
}

// SW-5 rev table: kindbasetable  hasoutnowed objtgt, also   same   disconnect( thenthenisbylang   code). 
func TestSW5UnseenGoalsFollowSameRouting(t *testing.T) {
	c := scLoadCorpus(t)
	_, m := scFixture(t)
	n := 0
	for _, s := range c.Samples {
		if !s.Assert || !s.Unseen {
			continue
		}
		n++
		got, ok := scPlan(t, "SW-5", s.ID, s.Goal, m)
		if !ok {
			continue
		}
		switch s.Class {
		case "capability_gap":
			if !got.Refused {
				t.Errorf("[SW-5] %s（unseen 不可达目标）未拒绝 → 判据可能只是按样本表查表", s.ID)
			}
			if len(got.Steps) != 0 {
				t.Errorf("[SW-5] %s 拒绝却仍给出步骤: %+v", s.ID, got.Steps)
			}
			if len(got.Missing) == 0 {
				t.Errorf("[SW-5] %s 拒绝了却没说缺什么", s.ID)
			} else if len(scOwners(strings.Join(got.Missing, " | "))) == 0 {
				t.Errorf("[SW-5] %s 的卡点没指到具体对象: %v", s.ID, got.Missing)
			}
		case "self_service":
			if got.Refused {
				t.Errorf("[SW-5] %s（unseen 可达目标）被拒绝 → 判据可能只是按样本表查表", s.ID)
			}
			if len(got.Steps) == 0 {
				t.Errorf("[SW-5][防空过] %s 计划为空", s.ID)
			}
			blob := strings.Join(got.Missing, " | ") + " || " + got.Reason
			if scHumanRouted.MatchString(blob) {
				t.Errorf("[SW-5] %s 自己能做却指向人: %q", s.ID, blob)
			}
		default:
			t.Errorf("[SW-5] unseen 样本 %s 的 class=%q 未定义断言", s.ID, s.Class)
		}
	}
	if n == 0 {
		t.Fatalf("[防空过] 没有 unseen 样本 → 反查表守卫失效")
	}
}
