//go:build vhsroute

// live_test.go --  endpoint data(env   ; key only process  /.env,       ). 
//
//	VHS_ROUTE_LIVE=1 ->  call close /api/route
//	VHS_L1_LIVE=1    ->  call deepseek-flash(L1)
package route

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"voicesign-harness/modelcenter"
)

func loadEnv(t *testing.T) string {
	t.Helper()
	if _, err := modelcenter.LoadDotEnv(filepath.Join("..", ".env")); err != nil && os.Getenv("AIOPS_KEY") == "" {
		t.Skipf("无 .env 且环境无 AIOPS_KEY（按纪律跳过）: %v", err)
	}
	key := os.Getenv("AIOPS_KEY")
	if key == "" {
		t.Skip("缺 AIOPS_KEY（按纪律跳过）")
	}
	return key
}

//  call close /api/route(L0     ). 
func TestLiveGatewayRoute(t *testing.T) {
	if os.Getenv("VHS_ROUTE_LIVE") != "1" {
		t.Skip("需要 VHS_ROUTE_LIVE=1（默认跳过）")
	}
	rc := &RouteClient{Endpoint: "https://aiops.peterzou.com/api/route", APIKey: loadEnv(t)}
	hits := 0
	for _, q := range []string{"部署到生产", "把这个文件删了", "爱ops", "翻译一下"} {
		name, ok, err := rc.Lookup(context.Background(), q)
		t.Logf("live /api/route q=%q → name=%q ok=%v err=%v", q, name, ok, err)
		if ok {
			hits++
		}
	}
	if hits == 0 {
		t.Errorf("真 /api/route 四次查询零命中 —— 解析或路径可能不对")
	}
}

//  call L1(deepseek-flash)--unique  objtgt. 
func TestLiveL1DeepseekFlash(t *testing.T) {
	if os.Getenv("VHS_L1_LIVE") != "1" {
		t.Skip("需要 VHS_L1_LIVE=1（默认跳过）")
	}
	key := loadEnv(t)
	_ = key
	cfg, err := modelcenter.Load(filepath.Join("..", "config", "model-center.json"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := modelcenter.NewRegistry(cfg)
	if err != nil {
		t.Skipf("缺少密钥环境变量（按纪律跳过）: %v", err)
	}
	r := &Router{L1: &L1Client{Registry: reg}, NeedsReasoning: true, Timeout: 40 * time.Second}
	d := r.Route(context.Background(), "先查库存再改报价最后提交", "多步目标：查库存→改报价→提交", Situation{})
	t.Logf("live L1: level=%s action=%s escalated=%v choice=%q degraded=%v(%s)",
		d.Level, d.Action, len(d.Ledger) > 0 && d.Ledger[len(d.Ledger)-1].Escalated, truncate(d.Choice, 80), d.Degraded, d.DegradedReason)
	if d.Level != LevelL1 || d.Degraded {
		t.Fatalf("L1 真调失败: %+v", d)
	}
}
