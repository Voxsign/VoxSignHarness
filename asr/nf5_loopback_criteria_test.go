package asr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// NF-5 · back   ( datafirst ): ** back ** require  bereject(403), back  require  . 
//
//  data(owner `docs/ approve  -produce and now-L01.md` §6 fix   3): 
//
//	"NF-5   : ASR serveservice**patchback   **(to line A   isLoopback +    127.0.0.1),  back reject. "
//
// to line A(`server/server.go:219-233`)  has form: 
//
//	token asempty ⇒ only allowback ly ( back  ⇒ 403"  token and base   ")
//
//    data(2026-10-03, see Issue #4): line B   `0.0.0.0:8123` time, 
// from LAN( back ) require `/v1/health`  to **200** ⇒ no   ⇒ base datacurbefore as ** **. 
func TestNF5NonLoopbackIsRejected(t *testing.T) {
	s := NewServer(nil)
	h := s.Handler()

	// ① back  ⇒   (use /v1/health,   dependency pipeline)
	reqLoop := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	reqLoop.RemoteAddr = "127.0.0.1:54321"
	wLoop := httptest.NewRecorder()
	h.ServeHTTP(wLoop, reqLoop)
	if wLoop.Code != http.StatusOK {
		t.Fatalf("[NF-5] 回环请求应 200，实际 %d", wLoop.Code)
	}

	// ②  back  ⇒ **   403**
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

// NF-5 patchfill:  back to hasendpointallreject( only /v1/health). 
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
