// task.go —— `POST /v1/task`：**只规划，不执行**（Lead 裁决 2026-10-03）。
//
// 边界（不许松）：
//
//	· 只规划，不执行 —— 响应恒带 execute:false，页面必须标注；
//	· document 只作规划输入，**不落盘到仓库**（本函数不写任何文件）；
//	· 返回：步骤 / 工具 / 依据 / **拒绝原因** / **该找谁** / Source / Degraded / 轨迹；
//	· **域门禁在规划期判断**（能力缺口/越域当场拒，而不是等到执行）。
package asr

import (
	"net/http"
	"strconv"

	"voicesign-harness/plan"
	"voicesign-harness/space"
	"voicesign-harness/tools"
)

// taskPlanStep 是给页面看的步骤（只读快照）。
type taskPlanStep struct {
	Index     int      `json:"index"`
	Tool      string   `json:"tool"`
	Caps      []string `json:"caps"`
	Action    string   `json:"action"`
	Output    string   `json:"output"`
	Why       string   `json:"why,omitempty"`
	Domain    string   `json:"domain,omitempty"`
	DependsOn []int    `json:"depends_on,omitempty"`
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var req struct {
		Task     string `json:"task"`
		Document string `json:"document"` // **仅作规划输入**，不落盘
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Task == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "task 不得为空"})
		return
	}
	// 能力清单以代码真值为准（contracts 目录取自 DataDir；缺失则用内置默认）。
	tr, err := tools.LoadContracts(s.DataDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "工具注册表: " + err.Error()})
		return
	}
	sr, err := space.Load(s.DataDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "域注册表: " + err.Error()})
		return
	}
	m := plan.ExportManifest(tr, sr)
	// document 作为**规划输入**参与目标文本（本轮仅拼接，不解析、不落盘）。
	goal := req.Task
	if req.Document != "" {
		goal = req.Task + "（附文档 " + strconv.Itoa(itoaLen(req.Document)) + " 字）"
	}
	p, err := plan.LocalPlanner{}.Plan(goal, m)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	steps := make([]taskPlanStep, 0, len(p.Steps))
	for i, st := range p.Steps {
		steps = append(steps, taskPlanStep{
			Index: i, Tool: st.Tool, Caps: st.Caps, Action: st.Action, Output: st.Output,
			Why: st.Why, Domain: st.Domain, DependsOn: st.DependsOn,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"execute": false, // ⚠️ 本轮只规划，不执行
		"goal":    p.Goal,
		"plan": map[string]any{
			"steps": steps, "refused": p.Refused, "reason": p.Reason,
			"missing": p.Missing, // 拒绝原因 + **该找谁**（owner 前缀）
			"source":  p.Source, "degraded": p.Degraded, "degraded_reason": p.DegradedReason,
			"considered": p.Considered, "wm": p.WM,
		},
		"note": "本轮只规划，不执行；document 仅作规划输入，不落盘。",
	})
}

func itoaLen(s string) int { return len([]rune(s)) }
