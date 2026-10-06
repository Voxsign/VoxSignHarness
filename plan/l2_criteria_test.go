//go:build vhsplanmodel

// l2_criteria_test.go -- Task A: L2 connectline(use**  type**  connectlineis  ,   etc owner   type). 
package plan

import (
	"context"
	"os"
	"testing"
)

// ① ruleform vs  typeform ⇒      same(use produceout same    connectline ). 
func TestL2ModelDiffersFromRule(t *testing.T) {
	_, m := fixture(t)
	rule, err := LocalPlanner{}.Plan("把这个项目里所有 TODO 整理成一份文档", m)
	if err != nil {
		t.Fatal(err)
	}
	//   typereturnback**  **  (andruleform   same)
	f := &fakeModel{out: `{"steps":[
	  {"tool":"search","caps":["text"],"params":{"pattern":"TODO"},"action":"搜索","output":"列表","why":"定位","domain":"project"},
	  {"tool":"file","caps":["read"],"params":{"path":"docs/TODO.md"},"action":"读现有汇总","output":"现有内容","why":"避免重复","domain":"project"},
	  {"tool":"file","caps":["write"],"params":{"path":"docs/TODO.md","content":"x"},"action":"写入","output":"文件","why":"落盘","domain":"project"}
	]}`}
	got, err := PlanWithL2(context.Background(), "把这个项目里所有 TODO 整理成一份文档", m, f, DefaultL2Model)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) == len(rule.Steps) {
		t.Fatalf("[L2] 模型式与规则式步数相同（%d）⇒ 接线可能没生效", len(rule.Steps))
	}
	if got.Source != "model" {
		t.Errorf("[L2] Source 应为 model，实际 %q", got.Source)
	}
	if f.calls != 1 {
		t.Errorf("[L2] 桩模型应被调用 1 次，实际 %d", f.calls)
	}
}

// ② L2  startuse(empty/off/nil)⇒ **and ruleform charseg  **(  because   modifychangedefault as). 
func TestL2DisabledBehavesExactlyLikeRule(t *testing.T) {
	_, m := fixture(t)
	rule, _ := LocalPlanner{}.Plan("帮我部署到生产服务器", m)
	for _, id := range []string{"", "off", "disabled"} {
		got, err := PlanWithL2(context.Background(), "帮我部署到生产服务器", m, &fakeModel{out: validModelPlan()}, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Source != rule.Source || got.Refused != rule.Refused ||
			len(got.Steps) != len(rule.Steps) || len(got.Missing) != len(rule.Missing) {
			t.Errorf("[L2] 未启用(%q)时行为应等同规则式: %+v vs %+v", id, got, rule)
		}
	}
	// model as nil samekind ruleform
	got, _ := PlanWithL2(context.Background(), "帮我部署到生产服务器", m, nil, DefaultL2Model)
	if got.Source != rule.Source || len(got.Missing) != len(rule.Missing) {
		t.Errorf("[L2] model=nil 应走规则式: %+v", got)
	}
}

// ③ L2   use(   )⇒    + tgtnote,   ,    . 
func TestL2FailureDegradesAndMarks(t *testing.T) {
	_, m := fixture(t)
	got, err := PlanWithL2(context.Background(), "把这个项目里所有 TODO 整理成一份文档", m,
		&fakeModel{err: os.ErrDeadlineExceeded}, DefaultL2Model)
	if err != nil {
		t.Fatalf("[L2] 不可用不得抛错: %v", err)
	}
	if !got.Degraded || got.DegradedReason == "" {
		t.Errorf("[L2] 降级未标注: %+v", got)
	}
	if len(got.Steps) == 0 && !got.Refused {
		t.Errorf("[L2] 降级后应有规则式结果: %+v", got)
	}
}

// ④  type id   : env >   file >  codedefault; defaultvaluei.e. Lead refer value. 
func TestL2ModelIsConfigurable(t *testing.T) {
	if DefaultL2Model != "deepseek-v4-pro" {
		t.Errorf("[L2] 默认模型应为 Lead 指定值，实际 %q", DefaultL2Model)
	}
	os.Unsetenv(EnvPlanL2Model)
	if got := L2ModelID("no-such-config.json"); got != DefaultL2Model {
		t.Errorf("[L2] 无配置时应回退默认: %q", got)
	}
	os.Setenv(EnvPlanL2Model, "some-other-model")
	defer os.Unsetenv(EnvPlanL2Model)
	if got := L2ModelID("no-such-config.json"); got != "some-other-model" {
		t.Errorf("[L2] env 未覆盖默认: %q", got)
	}
}
