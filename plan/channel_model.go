// channel_model.go -- pipe modelcenter   `default`   connectbecome PlanModel(  name  ). 
package plan

import (
	"context"
	"encoding/json"
	"strings"

	"voicesign-harness/modelcenter"
)

// ChannelPlanModel  ed typein    produceout    . 
//   namebycalluse    in(by ASR-MODEL-02 L1:   name  , bot  typefrom  read). 
type ChannelPlanModel struct {
	Registry *modelcenter.Registry
	Channel  modelcenter.Channel
}

// Propose send  rule  require, returnback typeorigstart base(bybase   ,    connect  ). 
func (c ChannelPlanModel) Propose(ctx context.Context, goal string, m Manifest) (string, error) {
	resp, err := c.Registry.Invoke(ctx, c.Channel, BuildPlanPrompt(goal, m))
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// BuildPlanPrompt pipeobjtgt + **  list**(unique value)  become showword. 
//   needrequire: only uselistin  ;   tothen refused + missing;  out JSON. 
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
