// channel_model.go —— 把 modelcenter 的 `default` 通道接成 PlanModel（通道名固定）。
package plan

import (
	"context"
	"encoding/json"
	"strings"

	"voicesign-harness/modelcenter"
)

// ChannelPlanModel 通过模型中心某通道产出候选计划。
// 通道名由调用方固定传入（按 ASR-MODEL-02 L1：通道名固定、底层模型从配置读）。
type ChannelPlanModel struct {
	Registry *modelcenter.Registry
	Channel  modelcenter.Channel
}

// Propose 发一次规划请求，返回模型原始文本（由本机复核，绝不直接执行）。
func (c ChannelPlanModel) Propose(ctx context.Context, goal string, m Manifest) (string, error) {
	resp, err := c.Registry.Invoke(ctx, c.Channel, BuildPlanPrompt(goal, m))
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// BuildPlanPrompt 把目标 + **能力清单**（唯一真值）渲染成提示词。
// 明确要求：只能用清单内工具；做不到就 refused + missing；输出 JSON。
func BuildPlanPrompt(goal string, m Manifest) string {
	type capView struct {
		Tool   string   `json:"tool"`
		Caps   []string `json:"caps"`
		Spaces []string `json:"allowed_spaces"`
	}
	views := make([]capView, 0, len(m.Tools))
	for _, c := range m.Tools {
		views = append(views, capView{Tool: c.Name, Caps: c.Caps, Spaces: c.AllowedSpaces})
	}
	b, _ := json.Marshal(views)
	var sb strings.Builder
	sb.WriteString("你是规划器。只允许使用下面能力清单里的工具；清单外的一律视为做不到。\n")
	sb.WriteString("能力清单(JSON): ")
	sb.Write(b)
	sb.WriteString("\n规则：\n1) 只能输出 JSON，不要解释文字；\n")
	sb.WriteString("2) 做不到时输出 {\"refused\":true,\"missing\":[\"...\"],\"reason\":\"...\"}；\n")
	sb.WriteString("3) 可达时输出 {\"steps\":[{\"tool\":..,\"caps\":[..],\"params\":{..},\"action\":..,\"output\":..,\"why\":..,\"domain\":..}]}；\n")
	sb.WriteString("4) 每步必须有 params / output / why（依据）；domain 必须在工具的 allowed_spaces 内。\n")
	sb.WriteString("目标：")
	sb.WriteString(goal)
	return sb.String()
}
