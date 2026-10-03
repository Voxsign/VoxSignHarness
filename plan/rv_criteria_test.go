//go:build vhsplan

// rv_criteria_test.go —— VHS-PLAN-001 §5：RV-1..RV-4（先红）。
//
// RV 测"规划完了之后再怎么改"：失败→复规、复规要说明变化、域门禁优先、不无限复规。
// 用一份**固定计划夹具**隔离 RV（不让 PL 未实现掩盖 RV 的结论）。
package plan

import (
	"errors"
	"reflect"
	"testing"
)

// rvFixturePlan 是一份结构合法的原计划（读 → 改 → 提交）。
func rvFixturePlan() Plan {
	return Plan{
		Goal: "先改这个文件，再提交",
		Steps: []Step{
			{Tool: "file", Caps: []string{"read"}, Params: map[string]string{"path": "a.go"}, Action: "读取 a.go", Output: "a.go 内容"},
			{Tool: "file", Caps: []string{"write"}, Params: map[string]string{"path": "a.go", "content": "..."}, Action: "写入 a.go", Output: "修改后的 a.go"},
			{Tool: "git", Caps: []string{"commit"}, Params: map[string]string{"args": "commit"}, Action: "提交改动", Output: "commit hash"},
		},
	}
}

func rvManifest(t *testing.T) Manifest {
	t.Helper()
	tr, sr := loadRegistries(t)
	return ExportManifest(tr, sr)
}

// RV-1 失败触发复规（不是盲重试同一步）。
func TestRV1FailureTriggersReplan(t *testing.T) {
	m := rvManifest(t)
	orig := rvFixturePlan()
	f := StepFailure{StepIndex: 2, Tool: "git", Caps: []string{"commit"}, Reason: "提交冲突"}
	got, err := Replan(orig, f, m, 1)
	if err != nil {
		t.Fatalf("[RV-1] 失败未触发复规（P1 桩，先红）: %v", err)
	}
	if len(got.Steps) == 0 {
		t.Fatal("[RV-1] 复规产出了空计划")
	}
	if reflect.DeepEqual(got.Steps, orig.Steps) {
		t.Error("[RV-1] 复规后计划与原计划完全相同（盲重试）")
	}
}

// RV-2 复规要说明变了什么（不得悄悄换一个计划）。
func TestRV2ReplanExplainsChanges(t *testing.T) {
	m := rvManifest(t)
	got, err := Replan(rvFixturePlan(), StepFailure{StepIndex: 2, Tool: "git", Caps: []string{"commit"}, Reason: "冲突"}, m, 1)
	if err != nil {
		t.Fatalf("[RV-2] 复规未实现（P1 桩，先红）: %v", err)
	}
	if len(got.Changes) == 0 {
		t.Error("[RV-2] 复规没有说明变化")
	}
}

// RV-3 域门禁优先：被域拒绝 → 计划作废，不得换说法绕过。
func TestRV3DomainDenialVoidsPlan(t *testing.T) {
	m := rvManifest(t)
	orig := rvFixturePlan()
	f := StepFailure{StepIndex: 1, Tool: "file", Caps: []string{"write"}, Reason: "域拒绝", DomainDenied: true, DeniedDomain: "vault-creds"}
	got, err := Replan(orig, f, m, 1)
	if err != nil {
		t.Fatalf("[RV-3] 复规未实现（P1 桩，先红）: %v", err)
	}
	if !got.Voided {
		t.Error("[RV-3] 被域门禁拒绝后计划未作废（可能换个说法绕过）")
	}
	for i, s := range got.Steps {
		if s.Tool == f.Tool && hasCap(s.Caps, "write") {
			t.Errorf("[RV-3] 复规后第 %d 步仍在做被拒绝的写动作（绕过门禁）", i)
		}
	}
}

// RV-4 不无限复规：超限必须如实报"做不到"。
func TestRV4BoundedReplans(t *testing.T) {
	m := rvManifest(t)
	f := StepFailure{StepIndex: 2, Tool: "git", Caps: []string{"commit"}, Reason: "一直失败"}
	_, err := Replan(rvFixturePlan(), f, m, MaxReplans+1)
	if !errors.Is(err, ErrUnachievable) {
		t.Errorf("[RV-4] 复规超限未如实报做不到（期望 ErrUnachievable，实际 %v；P1 桩为 ErrNotImplemented = 先红）", err)
	}
}
