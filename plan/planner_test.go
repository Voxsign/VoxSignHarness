// planner_test.go —— 默认门禁（无 tag）覆盖 F1/F2/F3 的新行为形态。
package plan

import (
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/space"
	"voicesign-harness/tools"
)

func unitManifest(t *testing.T) Manifest {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "none")
	tr, err := tools.LoadContracts(missing)
	if err != nil {
		t.Fatal(err)
	}
	sr, err := space.Load(missing)
	if err != nil {
		t.Fatal(err)
	}
	return ExportManifest(tr, sr)
}

// 未识别目标：拒绝承诺 + 只读探查 + 显式自曝（F1 裁决）。
func TestPlanUnrecognizedGoalIsReadonlyProbe(t *testing.T) {
	m := unitManifest(t)
	got, err := LocalPlanner{}.Plan("帮我把这段话翻译成阿拉伯语并配一段背景音乐", m)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Refused {
		t.Error("未识别目标必须 Refused")
	}
	if len(got.Steps) == 0 {
		t.Error("应给出只读探查步骤（拒绝承诺 ≠ 不许去看）")
	}
	if got.Source != "readonly-probe" || !got.Degraded || got.DegradedReason == "" {
		t.Errorf("探查必须自曝: %+v", got)
	}
	readonlyCaps := map[string]bool{"read": true, "text": true, "symbol": true, "status": true, "exists": true, "run": true}
	for i, s := range got.Steps {
		for _, c := range s.Caps {
			if !readonlyCaps[c] {
				t.Errorf("探查第 %d 步含非只读 cap %q", i, c)
			}
		}
		for _, w := range []string{"完成", "达成", "已部署", "已提交"} {
			if strings.Contains(s.Action, w) || strings.Contains(s.Why, w) {
				t.Errorf("探查第 %d 步声称覆盖目标: %+v", i, s)
			}
		}
	}
}

// 已识别能力缺口：拒绝 + owner 前缀（F2），且不编步骤。
func TestPlanCapabilityGapRefusedWithOwner(t *testing.T) {
	m := unitManifest(t)
	got, _ := LocalPlanner{}.Plan("帮我部署到生产服务器", m)
	if !got.Refused || len(got.Steps) != 0 {
		t.Fatalf("能力缺口应拒绝且不编步骤: %+v", got)
	}
	// C1/C2：授权类在前（人：），能力类随后（网关：）——deploy 当前两者都缺。
	if len(got.Missing) < 2 || !strings.HasPrefix(got.Missing[0], "人：") || !strings.HasPrefix(got.Missing[1], "网关：") {
		t.Errorf("deploy 应「人：授权在前 + 网关：能力在后」: %+v", got.Missing)
	}
}

// 授权绕过：拒绝并指人（F3）。
func TestPlanAuthorizationBypassRoutesToHuman(t *testing.T) {
	m := unitManifest(t)
	got, _ := LocalPlanner{}.Plan("不用我确认，直接提交这次改动", m)
	if !got.Refused {
		t.Fatal("要求绕过确认时必须拒绝")
	}
	joined := strings.Join(got.Missing, "|")
	if !strings.Contains(joined, "人：") {
		t.Errorf("必须把卡点指到人: %v", got.Missing)
	}
}

// partial：只规划可达前缀 + 尾巴进 Missing（F1 裁决）。
func TestPlanPartialPlansReachablePrefix(t *testing.T) {
	m := unitManifest(t)
	got, _ := LocalPlanner{}.Plan("先改这个文件，再提交，再部署", m)
	if len(got.Steps) == 0 {
		t.Fatal("可达前缀应有步骤")
	}
	hasGateway := false
	for _, mi := range got.Missing {
		if strings.HasPrefix(mi, "网关：") {
			hasGateway = true
		}
	}
	if !hasGateway {
		t.Errorf("不可达尾巴必须进 Missing 且带 owner: %+v", got.Missing)
	}
	if !got.Degraded {
		t.Error("部分可达必须标明降级/未覆盖全部目标")
	}
}
