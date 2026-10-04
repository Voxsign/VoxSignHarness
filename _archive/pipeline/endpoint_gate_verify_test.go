package pipeline

import (
	"os"
	"testing"
)

// 洞1 验证：证据门端点存在性检查——用真实需求规格 v3 + 无人工产物（2/7 端点）复现。
func TestEndpointGateRealCase(t *testing.T) {
	docBytes, err := os.ReadFile("../docs/ASR个性化后台服务-需求规格v3.md")
	if err != nil {
		t.Skipf("需求文档不可读: %v", err)
	}
	prodBytes, err := os.ReadFile("../harness-output/个性化后台实现/main.go")
	if err != nil {
		t.Skipf("无人工产物不可读: %v", err)
	}

	reqEP := endpointRefsOf(string(docBytes))
	prodEP := endpointHandlersOf(string(prodBytes))

	// 需求侧必须提取到 7 个 P0 端点。
	for _, want := range []string{"/v1/health", "/v1/dict", "/v1/term", "/v1/correct", "/v1/process", "/v1/feedback", "/v1/blacklist"} {
		if !reqEP[want] {
			t.Errorf("需求侧未提取到端点 %s", want)
		}
	}
	// 需求侧 7 个 P0 端点必须全部落在产物侧（2026-10-04 无人工成功版已 7/7 齐全）。
	// 缺口>0 意味着产物不完整 —— 洞1 判据5 抓现行。
	var missing []string
	for ep := range reqEP {
		if !prodEP[ep] {
			missing = append(missing, ep)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("期望 0 个端点缺口（产物应覆盖需求全部 7 端点），实得 %d: %v", len(missing), missing)
	}
	// 产物侧提取：7 个端点必须在。
	for _, want := range []string{"/v1/health", "/v1/process", "/v1/dict", "/v1/term", "/v1/correct", "/v1/feedback", "/v1/blacklist"} {
		if !prodEP[want] {
			t.Errorf("产物侧未提取到已注册端点 %s", want)
		}
	}
	t.Logf("PASS：需求 %d 端点，产物 %d 端点，缺口 %d → %v", len(reqEP), len(prodEP), len(missing), missing)
}

// 防回归：document 为空时判据 5 跳过（不误杀非实现场景）。
func TestEndpointGateNoDocument(t *testing.T) {
	ep := endpointRefsOf("")
	if len(ep) != 0 {
		t.Errorf("空文本不应提取端点")
	}
}
