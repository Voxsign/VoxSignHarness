// convert.go —— 薄客户端与 provider/ 的**协议互转**（防协议漂移）。
//
// 本包与 provider/ 是**两条客户端**，边界如下：
//
//	provider/         harness 主链的生产路径：重试、response_format、params 透传、
//	                  端点规范化（<base>/chat/completions）。**不要为别的线去改它。**
//	modelcenter/      三条通道（default/diagnose/learn）的配置与调用：C1–C5 fail-closed、
//	                  L2 写回令牌隔离、**按实测路径**（/api/model/chat）原样发起。
//	                  不引入重试/流式，保持最小、可审计。
//
// 两者**协议相同**（OpenAI 兼容：choices[].message.content / finish_reason / usage）。
// 本文件提供双向转换，使"协议漂移"变成**可机械判定的判据**：
// 一旦任一侧改了响应语义，round-trip 测试立刻变红。
package modelcenter

import (
	"voicesign-harness/contract"
	"voicesign-harness/provider"
)

// ToProviderResponse 把本包响应转成 provider 的响应类型。
// Channel / ModelID 是**通道层**属性，provider 信封没有对应字段，转换时丢弃（见文档）。
func ToProviderResponse(r Response) provider.ChatResponse {
	return provider.ChatResponse{
		Content:      r.Content,
		FinishReason: r.FinishReason,
		Usage: contract.Usage{
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.TotalTokens,
		},
	}
}

// FromProviderResponse 把 provider 响应转成本包响应类型。
// Channel 留空、ModelID 由调用方按配置回填（L5：以配置为准）。
func FromProviderResponse(r provider.ChatResponse) Response {
	return Response{
		Content:          r.Content,
		FinishReason:     r.FinishReason,
		PromptTokens:     r.Usage.PromptTokens,
		CompletionTokens: r.Usage.CompletionTokens,
		TotalTokens:      r.Usage.TotalTokens,
	}
}
