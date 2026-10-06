//go:build vhsplanmodel

// pm6_criteria_test.go -- PM-6   risk   first(Lead   :   typeto"   TODO"  run). 
package plan

import (
	"context"
	"strings"
	"testing"
)

func pm6Model() *fakeModel {
	return &fakeModel{out: `{"steps":[
	  {"tool":"run","caps":["exec"],"params":{"command":"grep -r TODO ."},"action":"用命令搜索 TODO","output":"TODO 列表","why":"能搜到","domain":"project"},
	  {"tool":"file","caps":["write"],"params":{"path":"docs/TODO.md","content":"x"},"action":"写入汇总","output":"文件","why":"落盘","domain":"project"}
	]}`}
}

// ①   classtask ⇒ **  **  run(   search): base     reject
func TestPM6SearchTaskMustNotUseRun(t *testing.T) {
	_, m := fixture(t)
	pl, err := ModelPlanner{Model: pm6Model()}.Plan("把这个项目里所有 TODO 整理成一份文档", m)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range pl.Steps {
		if s.Tool == "run" {
			t.Fatalf("[PM-6] 查找类任务仍选了 run（高风险）: %+v", pl.Steps)
		}
	}
	if !pl.Degraded || !strings.Contains(pl.DegradedReason, "PM-6") {
		t.Errorf("[PM-6] 违规未被本机拦下并留痕: degraded=%v reason=%q", pl.Degraded, pl.DegradedReason)
	}
}

// ②  torevexample: task**  needneed    ** ⇒  allow run(prevent"  forbid run" ed keep )
func TestPM6ExecTaskMayUseRun(t *testing.T) {
	_, m := fixture(t)
	execGoal := "执行一条命令跑一下安装"
	bad := reviewMinRisk(execGoal, []Step{{Tool: "run", Caps: []string{"run"}, Action: "执行", Output: "out", Why: "需要"}, {Tool: "file", Caps: []string{"write"}, Action: "写", Output: "f", Why: "落盘"}}, m)
	if len(bad) != 0 {
		t.Errorf("[PM-6 反例] 确需执行的任务被误拦: %v", bad)
	}
}

// ③   revexample: **use search done  ** ⇒    ed(    reject)
func TestPM6SearchToolAccepted(t *testing.T) {
	_, m := fixture(t)
	pl, err := ModelPlanner{Model: &fakeModel{out: `{"steps":[
	  {"tool":"search","caps":["text"],"params":{"pattern":"TODO"},"action":"搜索 TODO","output":"列表","why":"最轻","domain":"project"},
	  {"tool":"file","caps":["write"],"params":{"path":"docs/TODO.md","content":"x"},"action":"写入","output":"文件","why":"落盘","domain":"project"}
	]}`}}.Plan("把这个项目里所有 TODO 整理成一份文档", m)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Source != "model" || pl.Degraded {
		t.Errorf("[PM-6] 合规计划被误拒: source=%v degraded=%v(%s)", pl.Source, pl.Degraded, pl.DegradedReason)
	}
}

// ④ risktable   form: run=High, search=Low, and     as Unknown(  )
func TestPM6RiskTableIsExplicit(t *testing.T) {
	if ToolRiskOf("run") != RiskHigh || ToolRiskOf("search") != RiskLow {
		t.Errorf("[PM-6] 风险表不符: run=%v search=%v", ToolRiskOf("run"), ToolRiskOf("search"))
	}
	if ToolRiskOf("some-unknown-tool") != RiskUnknown {
		t.Errorf("[PM-6] 未登记工具应为 Unknown（不猜）")
	}
	_ = context.Background()
}
