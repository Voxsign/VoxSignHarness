// convert_test.go —— 协议互转判据（防 provider/ ↔ modelcenter/ 协议漂移）。
//
// 这是 Lead 明确要求补的一条：两条客户端可以并存，但**响应语义必须能互转**。
package modelcenter

import (
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/provider"
)

// TestProtocolRoundTrip：两侧响应类型必须能无损互转（共享字段逐字保留）。
func TestProtocolRoundTrip(t *testing.T) {
	orig := Response{
		Channel: ChannelDefault, ModelID: "deepseek-flash",
		Content: "收到", FinishReason: "stop",
		PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5,
	}
	back := FromProviderResponse(ToProviderResponse(orig))
	if back.Content != orig.Content || back.FinishReason != orig.FinishReason {
		t.Fatalf("内容/结束原因在互转中丢失: %+v", back)
	}
	if back.PromptTokens != orig.PromptTokens || back.CompletionTokens != orig.CompletionTokens || back.TotalTokens != orig.TotalTokens {
		t.Fatalf("usage 在互转中丢失: %+v", back)
	}
	// 通道层属性按文档丢弃（provider 信封没有对应字段）
	if back.Channel != "" || back.ModelID != "" {
		t.Fatalf("通道属性不应由 provider 信封带回: %+v", back)
	}
}

// TestProtocolFromProvider：provider 响应 → 本包响应，字段一一对应。
func TestProtocolFromProvider(t *testing.T) {
	pr := provider.ChatResponse{
		Content: "ok", FinishReason: "length",
		Usage: contract.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}
	got := FromProviderResponse(pr)
	if got.Content != "ok" || got.FinishReason != "length" || got.TotalTokens != 14 {
		t.Fatalf("转换不符: %+v", got)
	}
}
