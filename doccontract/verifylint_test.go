// verifylint_test.go -- V3-01…V3-08   as  . 
//
// overwrite: 
//  1. posexample:  rule  tgtapprove  -> 8  safety pass( rule   §3  showexample ). 
//  2.   revexample: V3-01…V3-08        fail useexample, anddisconnectlang"  onlyhas    ". 
//  3.    : threshold=unset  allowstore ( voicecalled met timesafetyed);   voicecalled met   fail. 
package doccontract

import (
	"fmt"
	"strings"
	"testing"
)

// vblock is    tgtapprove    ; yaml() pipe   become YAML    base. 
type vblock struct {
	owner          string
	claim          string
	method         string
	level          string
	kind           string
	ref            string
	threshold      string
	counterexample string
	verdictStates  string
	approver       string
	falsifier      string
}

// validBlock is   8  rulesafetyed baseline . 
func validBlock() vblock {
	return vblock{
		owner:          "builder-agent",
		claim:          "对任意 utterance u，若 confidence(u) < c.ConfThreshold，则 outcome.Ask 非空且该轮外部副作用数 = 0",
		method:         "test",
		level:          "E2",
		kind:           "external_artifact",
		ref:            "go.mod",
		threshold:      "副作用数 = 0（容差 0）；覆盖率 ≥ 45/45",
		counterexample: "输入『改一下』时系统产出 EDIT 且 Ask 为空 —— 观测特征：回执出现 BOUNDARY_VIOLATION",
		verdictStates:  "met, unverified, not_met",
		approver:       "reviewer-agent",
		falsifier:      "任何一条 confidence 低于阈值且 Ask 为空的用例即可证伪",
	}
}

// yaml   becomedefine  base; emptychar     become "" oremptyvalue, pos useat    charseg revexample. 
func (b vblock) yaml() string {
	return fmt.Sprintf(`owner: %s
verify:
  claim: %q
  method: %s
  evidence:
    level: %s
    kind: %s
    ref: %q
  threshold: %q
  counterexample: %q
  verdict_states: [%s]
  approver: %s
  falsifier: %q
`, b.owner, b.claim, b.method, b.level, b.kind, b.ref,
		b.threshold, b.counterexample, b.verdictStates, b.approver, b.falsifier)
}

// existsOnly is  use   resolver: only   "go.mod" store . 
func existsOnly(ref string) bool { return ref == "go.mod" }

// checkFixture use   resolver     8  rule. 
func checkFixture(t *testing.T, b vblock) *Report {
	t.Helper()
	rep, err := VerifyWith(b.yaml(), Options{Exists: existsOnly})
	if err != nil {
		t.Fatalf("解析验证标准块失败: %v\n输入:\n%s", err, b.yaml())
	}
	return rep
}

// TestVerifyCompliantBlockAllPass posexample:  rule  8  safety pass. 
func TestVerifyCompliantBlockAllPass(t *testing.T) {
	rep := checkFixture(t, validBlock())

	if got := len(rep.Results); got != 8 {
		t.Fatalf("规则条数 = %d，期望 8", got)
	}
	want := RuleIDs()
	for i, res := range rep.Results {
		if res.Rule != want[i] {
			t.Errorf("第 %d 条规则 ID = %s，期望 %s", i, res.Rule, want[i])
		}
		if res.Status != StatusPass {
			t.Errorf("%s 期望 pass，实得 %s（%s）", res.Rule, res.Status, res.Reason)
		}
	}
	if !rep.Pass() {
		t.Errorf("合规块应整体通过，失败规则: %v\n%s", rep.FailedRules(), rep)
	}
}

// TestVerifySpecExampleBlockAllPass rule   §3  showexample (patchon owner)  8  safetyed. 
//   sametime    tgt (claim / counterexample    )and  idnote   approver. 
func TestVerifySpecExampleBlockAllPass(t *testing.T) {
	rep, err := VerifyWith(specExampleBlock, Options{
		Exists: func(ref string) bool {
			return ref == "e2e/asrsim_test.go"
		},
	})
	if err != nil {
		t.Fatalf("解析规格书示例块失败: %v", err)
	}
	if !rep.Pass() {
		t.Fatalf("规格书示例块应全过，失败规则: %v\n%s", rep.FailedRules(), rep)
	}
}

// specExampleBlock    docs/VSL-v3-  tgtapprove .md §3, only top patch owner charseg
// (V3-07 needneed"same  in to owner"; rule  showexamplebase  write owner). 
const specExampleBlock = `owner: implementer-agent
purpose: 防止在模糊输入上猜错执行 —— 追问的代价远小于做错
priority: P0（安全红线，高于体验）
anti: 低置信时按"最近似规则"硬执行
cost: 多一次往返（用户多答一句）
correct: 置信 < 阈值 且 非 NOTE/ASK/REGISTER_TOOL 时，Ask 非空且不产生任何副作用
ask: "你是想让我做什么？请再说清楚一点"

# ---- v3 追加 ----
verify:
  claim: "对任意 utterance u，若 confidence(u) < c.ConfThreshold 且 intent(u) ∉ {NOTE,ASK,REGISTER_TOOL}，
          则 outcome.Ask ≠ '' 且该轮外部副作用数 = 0"
  method: test
  evidence:
    level: E2
    kind: external_artifact
    ref: "e2e/asrsim_test.go#TestASRSimFuzzyAcceptsIntentOrAsk"
  threshold: "副作用数 = 0（硬约束，容差 0）；覆盖率 ≥ 45/45 语料"
  counterexample: "输入『改一下』（无对象、无上下文）时系统产出 EDIT 且 Ask=''，
                   该轮进入执行通道 —— 观测特征：回执 result 出现 BOUNDARY_VIOLATION 或真实写盘"
  verdict_states: [met, unverified, not_met]
  approver: reviewer-agent（不得为实现者）
  falsifier: "任何一条 confidence<阈值 且 Ask='' 且产生副作用的用例，即可证伪"
`

// TestParseVerifyFoldsMultilineScalars rule  showexample   tgt   befinish  connect. 
func TestParseVerifyFoldsMultilineScalars(t *testing.T) {
	blk, err := ParseVerify(specExampleBlock)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	for _, want := range []string{"c.ConfThreshold", "则 outcome.Ask", "该轮外部副作用数 = 0"} {
		if !strings.Contains(blk.Claim, want) {
			t.Errorf("claim 折行拼接不完整，缺少 %q\n实得: %s", want, blk.Claim)
		}
	}
	for _, want := range []string{"观测特征", "BOUNDARY_VIOLATION", "真实写盘"} {
		if !strings.Contains(blk.Counterexample, want) {
			t.Errorf("counterexample 折行拼接不完整，缺少 %q\n实得: %s", want, blk.Counterexample)
		}
	}
	if len(blk.VerdictStates) != 3 {
		t.Errorf("verdict_states = %v，期望 3 项", blk.VerdictStates)
	}
	if blk.Owner != "implementer-agent" {
		t.Errorf("owner = %q，期望 implementer-agent", blk.Owner)
	}
}

// TestVerifyRuleCounterexamples   revexample:   rule    useexample, anddisconnectlang  onlyhas    . 
func TestVerifyRuleCounterexamples(t *testing.T) {
	cases := []struct {
		name   string
		rule   string
		mutate func(*vblock)
	}{
		{"V3-01 形容词 claim", "V3-01", func(b *vblock) { b.claim = "系统运行稳定高效" }},
		{"V3-01 空 claim", "V3-01", func(b *vblock) { b.claim = "" }},
		{"V3-01 英文模糊词", "V3-01", func(b *vblock) { b.claim = "the system is stable and fast" }},

		{"V3-02 method 非枚举", "V3-02", func(b *vblock) { b.method = "vibes" }},
		{"V3-02 method 为空", "V3-02", func(b *vblock) { b.method = "" }},

		{"V3-03 E0 伴随 met", "V3-03", func(b *vblock) {
			b.level = "E0"
			b.verdictStates = "met, unverified, not_met"
		}},
		{"V3-03 level 非枚举", "V3-03", func(b *vblock) { b.level = "E9" }},
		{"V3-03 E0 缺省三态含 met", "V3-03", func(b *vblock) {
			b.level = "E0"
			b.verdictStates = ""
		}},

		{"V3-04 ref 为空", "V3-04", func(b *vblock) { b.ref = "" }},
		{"V3-04 ref 不存在", "V3-04", func(b *vblock) { b.ref = "no/such/artifact.md" }},
		{"V3-04 ref 占位符", "V3-04", func(b *vblock) { b.ref = "<文件/产物路径或 ID>" }},

		{"V3-05 unset 却声称 met", "V3-05", func(b *vblock) { b.threshold = "unset" }},
		{"V3-05 无数字", "V3-05", func(b *vblock) { b.threshold = "尽可能快" }},
		{"V3-05 threshold 为空", "V3-05", func(b *vblock) { b.threshold = "" }},

		{"V3-06 counterexample 为 claim 简单取反", "V3-06", func(b *vblock) {
			b.counterexample = "不" + b.claim
		}},
		{"V3-06 counterexample 为空", "V3-06", func(b *vblock) { b.counterexample = "" }},
		{"V3-06 counterexample 等于 claim", "V3-06", func(b *vblock) {
			b.counterexample = b.claim
		}},

		{"V3-07 approver 与 owner 同一", "V3-07", func(b *vblock) { b.approver = b.owner }},
		{"V3-07 approver 为空", "V3-07", func(b *vblock) { b.approver = "" }},
		{"V3-07 缺 owner 无法比对", "V3-07", func(b *vblock) { b.owner = "" }},

		{"V3-08 falsifier 为空", "V3-08", func(b *vblock) { b.falsifier = "" }},
	}

	covered := map[string]int{}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := validBlock()
			tc.mutate(&b)
			rep := checkFixture(t, b)

			failed := rep.FailedRules()
			if len(failed) != 1 || failed[0] != tc.rule {
				t.Fatalf("期望恰好 %s 失败，实得 %v\n%s", tc.rule, failed, rep)
			}
		})
		covered[tc.rule]++
	}

	for _, rule := range RuleIDs() {
		if covered[rule] == 0 {
			t.Errorf("规则 %s 没有对应的 fail 用例（逐条反例要求每条至少一个）", rule)
		}
	}
}

// TestVerifyThresholdUnsetTolerance to rule   §4 / §7.2: 
// threshold=unset  allowstore ; but  sametimevoicecalled met then   fail. 
func TestVerifyThresholdUnsetTolerance(t *testing.T) {
	t.Run("unset 允许存在且不声称 met", func(t *testing.T) {
		b := validBlock()
		b.threshold = "unset"
		b.verdictStates = "unverified, not_met"
		rep := checkFixture(t, b)
		if !rep.Pass() {
			t.Fatalf("threshold=unset 且未声称 met 应全过，失败规则: %v\n%s", rep.FailedRules(), rep)
		}
	})

	t.Run("unset 同时声称 met 必须 fail", func(t *testing.T) {
		b := validBlock()
		b.threshold = "unset"
		b.verdictStates = "met, unverified, not_met"
		rep := checkFixture(t, b)
		failed := rep.FailedRules()
		if len(failed) != 1 || failed[0] != "V3-05" {
			t.Fatalf("期望恰好 V3-05 失败，实得 %v\n%s", failed, rep)
		}
	})
}

// TestVerifyEvidenceRefAgainstFilesystem V3-04    file  (Root refermoduleroot). 
func TestVerifyEvidenceRefAgainstFilesystem(t *testing.T) {
	root := moduleRoot(t)

	t.Run("真实存在的产物通过", func(t *testing.T) {
		b := validBlock()
		b.ref = "go.mod"
		rep, err := VerifyWith(b.yaml(), Options{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		if !rep.Pass() {
			t.Fatalf("go.mod 真实存在，应全过，失败规则: %v\n%s", rep.FailedRules(), rep)
		}
	})

	t.Run("不存在的产物 V3-04 fail", func(t *testing.T) {
		b := validBlock()
		b.ref = "docs/这条产物不存在.md"
		rep, err := VerifyWith(b.yaml(), Options{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		failed := rep.FailedRules()
		if len(failed) != 1 || failed[0] != "V3-04" {
			t.Fatalf("期望恰好 V3-04 失败，实得 %v\n%s", failed, rep)
		}
	})
}

// TestVerifyMissingVerifyBlock  seg has verify  time, 8  safety  fail(      ). 
func TestVerifyMissingVerifyBlock(t *testing.T) {
	rep, err := VerifyWith("purpose: 只有 v2 字段\npriority: P0\n", Options{Exists: existsOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.FailedRules()) != 8 {
		t.Fatalf("缺少 verify 块时应 8 条全 fail，实得 %v\n%s", rep.FailedRules(), rep)
	}
}

// TestVerifyBareVerifyBlockWithoutWrapper  allow inbase thenis verify  (no verify:   ). 
func TestVerifyBareVerifyBlockWithoutWrapper(t *testing.T) {
	text := `owner: builder-agent
claim: 对任意 utterance u，若 confidence(u) < c.ConfThreshold，则 outcome.Ask 非空
method: test
evidence:
  level: E2
  kind: external_artifact
  ref: "go.mod"
threshold: "副作用数 = 0（容差 0）"
counterexample: 输入『改一下』时产出 EDIT —— 观测特征：回执出现 BOUNDARY_VIOLATION
verdict_states: [met, unverified, not_met]
approver: reviewer-agent
falsifier: 任何一条 Ask 为空且产生副作用的用例即可证伪
`
	rep, err := VerifyWith(text, Options{Exists: existsOnly})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Pass() {
		t.Fatalf("裸 verify 块应全过，失败规则: %v\n%s", rep.FailedRules(), rep)
	}
}

// TestVerifyBlockListForm verdict_states use listtablewrite also   resolve . 
func TestVerifyBlockListForm(t *testing.T) {
	text := strings.Replace(
		validBlock().yaml(),
		"  verdict_states: [met, unverified, not_met]",
		"  verdict_states:\n    - met\n    - unverified\n    - not_met",
		1,
	)
	blk, err := ParseVerify(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(blk.VerdictStates) != 3 {
		t.Fatalf("块列表解析失败: %v", blk.VerdictStates)
	}
	rep, err := VerifyWith(text, Options{Exists: existsOnly})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Pass() {
		t.Fatalf("块列表写法应全过，失败规则: %v\n%s", rep.FailedRules(), rep)
	}
}

// TestParseVerifyRejectsTabIndent restricttable       , but is  resolve become close . 
func TestParseVerifyRejectsTabIndent(t *testing.T) {
	_, err := ParseVerify("verify:\n\tclaim: x\n")
	if err == nil {
		t.Fatal("制表符缩进应返回解析错误")
	}
}
