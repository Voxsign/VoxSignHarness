package asr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// NF-5 · 回环鉴权（判据先红）：**非回环**请求必须被拒（403），回环请求放行。
//
// 依据（Peter `docs/校准报告-产品与实现-L01.md` §6 修订项 3）：
//
//	「NF-5 鉴权：ASR 服务**补回环鉴权**（对齐线 A 的 isLoopback + 绑定 127.0.0.1），非回环拒绝。」
//
// 对齐线 A（`server/server.go:219-233`）的既有模式：
//
//	token 为空 ⇒ 只允许回环地址（非回环 ⇒ 403「缺 token 且非本机访问」）
//
// 真跑证据（2026-10-03，见 Issue #4）：线 B 绑 `0.0.0.0:8123` 时，
// 从 LAN（非回环）请求 `/v1/health` 得到 **200** ⇒ 无鉴权 ⇒ 本判据当前应为 **红**。
func TestNF5NonLoopbackIsRejected(t *testing.T) {
	s := NewServer(nil)
	h := s.Handler()

	// ① 回环 ⇒ 放行（用 /v1/health，它不依赖 pipeline）
	reqLoop := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	reqLoop.RemoteAddr = "127.0.0.1:54321"
	wLoop := httptest.NewRecorder()
	h.ServeHTTP(wLoop, reqLoop)
	if wLoop.Code != http.StatusOK {
		t.Fatalf("[NF-5] 回环请求应 200，实际 %d", wLoop.Code)
	}

	// ② 非回环 ⇒ **必须 403**
	for _, addr := range []string{"192.168.8.129:54321", "10.0.0.7:1234", "[2001:db8::1]:9999"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
		req.RemoteAddr = addr
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("[NF-5] 非回环 %s 应 **403**，实际 **%d**（body=%q）"+
				" ⇒ ASR 服务对非本机开放，无鉴权",
				addr, w.Code, strings.TrimSpace(w.Body.String()))
		}
	}
}

// NF-5 补充：非回环对所有端点都拒（不只 /v1/health）。
func TestNF5NonLoopbackRejectedOnAllEndpoints(t *testing.T) {
	s := NewServer(nil)
	h := s.Handler()
	for _, p := range []string{"/v1/health", "/v1/process", "/v1/dictionary", "/v1/feedback", "/v1/lexicon"} {
		req := httptest.NewRequest(http.MethodPost, p, strings.NewReader("{}"))
		req.RemoteAddr = "192.168.8.129:55555"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("[NF-5] 非回环请求 %s 应 403，实际 %d", p, w.Code)
		}
	}
}
