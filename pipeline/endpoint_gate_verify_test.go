package pipeline

import (
	"os"
	"testing"
)

//  1   :  data endpointstore ity  --use  needrequirerule  v3 + nohumanartifact(2/7 endpoint) now. 
func TestEndpointGateRealCase(t *testing.T) {
	docBytes, err := os.ReadFile("../docs/ASR个性化后台服务-需求规格v3.md")
	if err != nil {
		t.Skipf("需求文档不可读: %v", err)
	}
	// 2026-10-04: artifactmodifyasin  fixture(2/7 endpoint)--   artifactalreadyis    7/7, 
	//   dependency   file  artifact  but  ;  1  now scenario  as"needrequire 7 endpoint vs artifact 2 endpoint". 
	prodBytes := []byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {})
	http.HandleFunc("/v1/process", func(w http.ResponseWriter, r *http.Request) {})
}
`)

	reqEP := endpointRefsOf(string(docBytes))
	prodEP := endpointHandlersOf(string(prodBytes))

	// needrequireside   getto 7   P0 endpoint. 
	for _, want := range []string{"/v1/health", "/v1/dict", "/v1/term", "/v1/correct", "/v1/process", "/v1/feedback", "/v1/blacklist"} {
		if !reqEP[want] {
			t.Errorf("需求侧未提取到端点 %s", want)
		}
	}
	// nohumanartifactobjbeforeonlynote  2   -> diff      5    ( 1  now ). 
	var missing []string
	for ep := range reqEP {
		if !prodEP[ep] {
			missing = append(missing, ep)
		}
	}
	if len(missing) != 5 {
		t.Fatalf("期望 5 个端点缺口，实得 %d: %v", len(missing), missing)
	}
	// artifactside get: health+process    . 
	for _, want := range []string{"/v1/health", "/v1/process"} {
		if !prodEP[want] {
			t.Errorf("产物侧未提取到已注册端点 %s", want)
		}
	}
	t.Logf("PASS：需求 %d 端点，产物 %d 端点，缺口 %d → %v", len(reqEP), len(prodEP), len(missing), missing)
}

// preventback : document asemptytime data 5  ed(     now scenario). 
func TestEndpointGateNoDocument(t *testing.T) {
	ep := endpointRefsOf("")
	if len(ep) != 0 {
		t.Errorf("空文本不应提取端点")
	}
}
