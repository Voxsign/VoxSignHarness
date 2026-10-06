// convert_test.go --      data(prevent provider/ ↔ modelcenter/     ). 
//
//  is Lead   needrequirepatch   :   clientuserend byandstore, but**  semantic     **. 
package modelcenter

import (
	"testing"

	"voicesign-harness/contract"
	"voicesign-harness/provider"
)

// TestProtocolRoundTrip:  side  classtype   no   (  charseg charkeep ). 
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
	//     ityby    (provider    hasto charseg)
	if back.Channel != "" || back.ModelID != "" {
		t.Fatalf("通道属性不应由 provider 信封带回: %+v", back)
	}
}

// TestProtocolFromProvider: provider    -> this package  , charseg  to . 
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
