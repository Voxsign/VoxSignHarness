//go:build vhs002live

// intent_live_test.go --   type bot **    **(   tag,  continue  vhs002    noteinopenclose). 
//
//   : VHS_MODEL_LIVE=1 go test -tags vhs002live ./asr -run TestLiveIntent
// key onlyfromprocess  /.env read;   key -> skip( in ). 
package asr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/modelcenter"
)

func TestLiveIntentModelFallbackAndRealTimeout(t *testing.T) {
	if os.Getenv("VHS_MODEL_LIVE") != "1" {
		t.Skip("需要 VHS_MODEL_LIVE=1（默认跳过）")
	}
	if _, err := modelcenter.LoadDotEnv(filepath.Join("..", ".env")); err != nil && os.Getenv("AIOPS_KEY") == "" {
		t.Skipf("无 .env 且环境无 AIOPS_KEY（按纪律跳过）: %v", err)
	}
	cfg, err := modelcenter.Load(filepath.Join("..", "config", "model-center.json"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := modelcenter.NewRegistry(cfg)
	if err != nil {
		t.Skipf("缺少密钥环境变量（按纪律跳过）: %v", err)
	}

	// ①    bot: low-confidence base ->   call default   and  itsclassify. 
	got := ClassifyIntentWith(context.Background(), "嗯这个东西吧", reg, 40*time.Second)
	if got.Degraded {
		t.Fatalf("真实兜底失败(降级) = 链路未验证: %s", got.DegradedReason)
	}
	if !knownIntents[got.Type] {
		t.Fatalf("兜底返回非法类别: %+v", got)
	}
	t.Logf("live 兜底 OK: type=%s confidence=%.2f", got.Type, got.Confidence)

	// ② **   timepath**: give 1ms  time,   sendraisecalluseand   time ->    fail-open   . 
	start := time.Now()
	to := ClassifyIntentWith(context.Background(), "嗯这个东西吧", reg, time.Millisecond)
	elapsed := time.Since(start)
	if !to.Degraded {
		t.Fatalf("真实超时未降级（结构绿≠能力验）: %+v", to)
	}
	if to.Type == "" {
		t.Fatal("超时后必须返回本地结果")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("超时路径未及时返回: %v", elapsed)
	}
	if !strings.Contains(to.DegradedReason, "超时") && !strings.Contains(to.DegradedReason, "deadline") && !strings.Contains(to.DegradedReason, "context") {
		t.Logf("降级原因（供核对）: %s", to.DegradedReason)
	}
	t.Logf("live 真实超时降级 OK：%v，原因=%s", elapsed, to.DegradedReason)

	// ③ occurproduce time(3s)  : VHS_INTENT_3S=1 time  (default edbykeepkeep live fast ). 
	if os.Getenv("VHS_INTENT_3S") == "1" {
		start3 := time.Now()
		p3 := ClassifyIntentWith(context.Background(), "嗯这个东西吧", reg, DefaultIntentTimeout)
		t.Logf("live 3s 路径：elapsed=%v degraded=%v type=%s reason=%q",
			time.Since(start3), p3.Degraded, p3.Type, p3.DegradedReason)
	}
}
