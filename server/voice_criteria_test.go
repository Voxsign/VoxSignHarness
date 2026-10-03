// voice_criteria_test.go —— /v1/voice 常驻判据。
//
// ⚠️ 层次标签（诚实）：本判据走 **Server.Handler()（与 Start 同一份装配）**，
// 但用 **httptest**（进程内），**不是"起真进程"的真装配级**。
// 真装配级证据见 Issue #4 的手工真跑（起 `serve` + 桩 ASR：202 / 503+degraded / ASK 不执行）。
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"voicesign-harness/config"
	"voicesign-harness/pipeline"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{} // token 空 ⇒ 仅回环可访问（httptest 即回环）
	srv := New(cfg, &pipeline.Options{})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func asrStub(t *testing.T, resp map[string]any) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(s.Close)
	return s
}

func postVoice(t *testing.T, base, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(base+"/v1/voice", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// ① 「ASR 在 + 意图可执行 ⇒ 202 + task_id」**不在此常驻判据中**：
// 它需要 main.go 同款的完整 pipeline.Options（空 Options 会让 pipeline.Run 在后台 goroutine 空指针）。
// 该路径的证据是 **真装配级手工真跑**（起 serve + 桩 ASR ⇒ 202 + task_id，见 Issue #4）。

// ② 线 B 不可达 ⇒ 503 + degraded，且**不创建任务**（不许静默降级）
func TestVoiceASRUnreachableIsExplicit(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	dead.Close() // 立刻关 ⇒ 不可达
	t.Setenv("VHS_ASR_ENDPOINT", dead.URL)
	code, out := postVoice(t, testServer(t).URL, `{"text":"查一下库存"}`)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("[voice②] 期望 503，实际 %d: %v", code, out)
	}
	if out["degraded"] != true {
		t.Errorf("[voice②] 未标 degraded: %v", out)
	}
	if _, has := out["task_id"]; has {
		t.Errorf("[voice②] 不可达却创建了任务（静默降级）: %v", out)
	}
}

// ③ 红线：ASK / need_disambiguate / ask 非空 ⇒ **绝不执行**
func TestVoiceAskNeverExecutes(t *testing.T) {
	for i, resp := range []map[string]any{
		{"type": "ASK", "need_disambiguate": true},
		{"type": "EDIT", "need_disambiguate": true},
		{"type": "EDIT", "ask": "你指的是哪个文件？"},
	} {
		a := asrStub(t, resp)
		os.Setenv("VHS_ASR_ENDPOINT", a.URL)
		code, out := postVoice(t, testServer(t).URL, `{"text":"把那个改了"}`)
		os.Unsetenv("VHS_ASR_ENDPOINT")
		if code != http.StatusOK {
			t.Fatalf("[voice③-%d] 期望 200（不执行），实际 %d: %v", i, code, out)
		}
		if out["executed"] != false {
			t.Errorf("[voice③-%d] ASK 却执行了（红线）: %v", i, out)
		}
		if _, has := out["task_id"]; has {
			t.Errorf("[voice③-%d] ASK 却创建了任务: %v", i, out)
		}
	}
}
