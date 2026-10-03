// l1.go —— L1 慢通道客户端（deepseek-flash），**唯一升级目标**。
//
// 升级触发器只有两个（VHS-FASTSLOW-001 修订后）：
//
//	① JEV **不可用**（超时/非 200/格式错 —— 技术失败，不是"判了但说不清"）
//	② 调用方声明**需要多步推理**
//
// `choice=ambiguous` **不在此列**：那是 JEV 给出的正确答案"说不清" ⇒ 回问用户。
package route

import (
	"context"

	"voicesign-harness/modelcenter"
)

// L1Client 用 modelcenter 的 default 通道（deepseek-flash）实现慢通道。
type L1Client struct {
	Registry *modelcenter.Registry
	Channel  modelcenter.Channel
}

// Complete 调一次慢通道。
func (c *L1Client) Complete(ctx context.Context, prompt string) (string, error) {
	ch := c.Channel
	if ch == "" {
		ch = modelcenter.ChannelDefault
	}
	resp, err := c.Registry.Invoke(ctx, ch, prompt)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
