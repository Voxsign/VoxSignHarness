//go:build vhsroute

// jev_live_test.go —— 真 JEV 实跑（env 守卫；key 只在进程环境/.env，不打印不落盘）。
package route

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"voicesign-harness/modelcenter"
)

func TestLiveJEVRouting(t *testing.T) {
	if os.Getenv("VHS_JEV_LIVE") != "1" {
		t.Skip("需要 VHS_JEV_LIVE=1（默认跳过）")
	}
	if _, err := modelcenter.LoadDotEnv(filepath.Join("..", ".env")); err != nil && os.Getenv("AIOPS_KEY") == "" {
		t.Skipf("无 .env 且环境无 AIOPS_KEY（按纪律跳过）: %v", err)
	}
	key := os.Getenv("AIOPS_KEY")
	if key == "" {
		t.Skip("缺 AIOPS_KEY（按纪律跳过）")
	}
	jev := &JEVClient{Endpoint: "https://aiops.peterzou.com/api/decide", APIKey: key, Kind: KindPermission}
	r := &Router{Hot: nil, JEV: jev, Timeout: 10 * time.Second}

	// 权限类：按实测结论，选项要表达成 candidates（constraints 放 why）
	d := r.Route(context.Background(), "能不能写入 vault-creds",
		"能否写入 vault-creds？",
		Situation{
			Candidates: []Candidate{
				{ID: "allow", Why: "域=project"},
				{ID: "deny", Why: "域=vault-creds + file.write 不在允许集"},
			},
			Constraints: []string{"域=vault-creds 只读"},
			Memory:      []string{"用户最近常提 vault-creds"},
		})
	t.Logf("live JEV: level=%s action=%s choice=%q conf=%.2f model=%s reason=%q degraded=%v(%s)",
		d.Level, d.Action, d.Choice, d.Confidence, d.ModelID, d.Reason, d.Degraded, d.DegradedReason)
	if d.Degraded {
		t.Fatalf("JEV 应可用（Lead 实测通过）：%s", d.DegradedReason)
	}
	if d.Action != ActionAnswer && d.Action != ActionAskUser {
		t.Errorf("动作异常: %+v", d)
	}
}
