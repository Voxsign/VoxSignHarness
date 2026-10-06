// live_test.go --   close  (default ed; VHS_MODEL_LIVE=1 and .env store timeonly ). 
//
//   : key onlyfrom .env /   change read,     outnow   key charface ;      back  key. 
package modelcenter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveDefaultChannelSmoke(t *testing.T) {
	if os.Getenv("VHS_MODEL_LIVE") != "1" {
		t.Skip("需要 VHS_MODEL_LIVE=1 才跑真网关（默认跳过，避免测试依赖外网/密钥）")
	}
	if _, err := LoadDotEnv(filepath.Join("..", ".env")); err != nil && !os.IsNotExist(err) {
		t.Skipf(".env 不可读（按纪律跳过，不内置 key）: %v", err)
	}
	cfg, err := Load(filepath.Join("..", "config", "model-center.json"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(cfg)
	if err != nil {
		t.Skipf("缺少密钥环境变量（按纪律跳过）: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := reg.Invoke(ctx, ChannelDefault, "只回复两个字：收到")
	if err != nil {
		t.Fatalf("default 通道调用失败: %v", err)
	}
	if resp.Content == "" {
		t.Error("live 响应内容为空")
	}
	if resp.ModelID != "deepseek-flash" {
		t.Errorf("model_id=%q，期望 deepseek-flash（ASR-EXT-006 §2）", resp.ModelID)
	}
	t.Logf("live default 通道 OK：model=%s finish=%s tokens=%d content_len=%d",
		resp.ModelID, resp.FinishReason, resp.TotalTokens, len(resp.Content))
}
