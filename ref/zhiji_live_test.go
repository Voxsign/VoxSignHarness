//go:build vhsref

// zhiji_live_test.go --   clientuserend** endpoint** data(VHS_ZHIJI_LIVE=1; key only   /.env). 
// curl   != clientuserend code (" raise etc "  5  ),  by     . 
package ref

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"voicesign-harness/modelcenter"
)

func TestLiveZhijiEvents(t *testing.T) {
	if os.Getenv("VHS_ZHIJI_LIVE") != "1" {
		t.Skip("需要 VHS_ZHIJI_LIVE=1（默认跳过）")
	}
	if _, err := modelcenter.LoadDotEnv(filepath.Join("..", ".env")); err != nil && os.Getenv("AIOPS_KEY") == "" {
		t.Skipf("无 .env 且环境无 AIOPS_KEY（按纪律跳过）: %v", err)
	}
	key := os.Getenv("AIOPS_KEY")
	if key == "" {
		t.Skip("缺 AIOPS_KEY（按纪律跳过）")
	}
	c := &ZhijiClient{Endpoint: "https://aiops.example.com/api/zhiji", APIKey: key}
	evs, err := c.Events(context.Background())
	if err != nil {
		t.Fatalf("真调知己失败: %v", err)
	}
	if len(evs) == 0 {
		t.Fatalf("知己返回 0 条事件（期望 count>0）")
	}
	// tail charnodeis**already  state**, resolve pathalready clientuserendin  ;   disconnectlangcharseg use
	if evs[0].Text == "" {
		t.Errorf("首条事件 text 为空: %+v", evs[0])
	}
	t.Logf("live 知己: %d 条事件；首条 source=%q text=%q", len(evs), evs[0].Source, truncate80(evs[0].Text))
}

func truncate80(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:80]
}
