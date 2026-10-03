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
	"strings"

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
	// ② **让文档真的被读**：结构 + 关键词进 considered / WM（**不把全文塞进提示词**）。
	goal := req.Task // 注意：goal **不再**写"附文档 N 字"（那是"看起来读了"）
	p, err := plan.LocalPlanner{}.Plan(goal, m)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	doc := summarizeDocument(req.Document)
	if doc.TooLarge {
		p.Degraded = true
		p.DegradedReason = "文档过大（>" + strconv.Itoa(maxDocRunes) + " 字），已截断摘要，未全文参与规划"
	}
	if doc.Read {
		// ① 结构进 considered
		p.Considered = append([]plan.ConsideredItem{{
			Element: doc.Describe(), Source: "<document>", Inferred: false,
		}}, p.Considered...)
		// ③ 任务与文档的关键词命中可见
		for _, kw := range doc.HitKeywords(req.Task) {
			p.Considered = append(p.Considered, plan.ConsideredItem{
				Element: "document_hit:" + kw, Source: "<document>", Inferred: true,
			})
		}
		// ② 文档实体进活跃实体板口径：w_used 因文档而变（超容量则留痕）
		added := 0
		for _, e := range doc.Entities() {
			if p.WM.Used+added >= p.WM.Max {
				p.WM.Drop++
				p.WM.DropTrace = append(p.WM.DropTrace, "document:"+e)
				continue
			}
			added++
			p.Considered = append(p.Considered, plan.ConsideredItem{
				Element: "document_entity:" + e, Source: "<document>", Inferred: true,
			})
		}
		p.WM.Used += added

		// #2-c ① **文档影响步骤**：条目数 ⇒ 批处理粒度（规则式能做的那一档）。
		// ② 反例防线：批数**只由条目数**决定 ⇒ 内容等价但措辞不同的文档得到相同 steps。
		driver := doc.Items
		driverName := "条目"
		if doc.TODOs > driver {
			driver, driverName = doc.TODOs, "TODO"
		}
		if driver == doc.Items && doc.TODOs == doc.Items {
			driverName = "条目/TODO"
		}
		batches := (driver + batchSizeItems - 1) / batchSizeItems
		_ = driverName
		if batches > 1 {
			p.Steps = expandBatchedSteps(p.Steps, batches)
			p.WM.ChainLen = len(p.Steps) // ③ 展开后必须与 steps 一致（否则那个数字是假的）
			p.Considered = append(p.Considered, plan.ConsideredItem{
				Element: "document_batches: " + strconv.Itoa(batches) + "（" + driverName +
					" " + strconv.Itoa(driver) + " 条 / 每批 " + strconv.Itoa(batchSizeItems) + "）",
				Source: "<document>", Inferred: true,
			})
		}
	}
	steps := make([]taskPlanStep, 0, len(p.Steps))
	for i, st := range p.Steps {
		steps = append(steps, taskPlanStep{
			Index: i, Tool: st.Tool, Caps: st.Caps, Action: st.Action, Output: st.Output,
			Why: st.Why, Domain: st.Domain, DependsOn: st.DependsOn,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"execute":       false,    // ⚠️ 本轮只规划，不执行
		"document_read": doc.Read, // 空文档/未读 ⇒ false + note（不许静默当没有）
		"document_note": doc.Note,
		"document": map[string]any{
			"lines": doc.Lines, "items": doc.Items, "todos": doc.TODOs,
			"keywords": doc.Keywords, "too_large": doc.TooLarge,
		},
		"goal": p.Goal,
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

// batchSizeItems 是每个"批处理"步骤覆盖的文档条目数。
// ⚠️ **UNVALIDATED**（我取的 10，未标定）。
const batchSizeItems = 10

// expandBatchedSteps 把计划里**最后一个写文件步骤**按批数展开（其余步骤不变）。
// 规则式规划器能表达的粒度就到这里：**批数由文档条目数决定**。
func expandBatchedSteps(steps []plan.Step, batches int) []plan.Step {
	idx := -1
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Tool == "file" && containsCap(steps[i].Caps, "write") {
			idx = i
			break
		}
	}
	if idx < 0 || batches < 2 {
		return steps
	}
	orig := steps[idx]
	out := append([]plan.Step(nil), steps[:idx]...)
	prev := idx - 1
	for b := 1; b <= batches; b++ {
		step := orig
		if b > 1 {
			step.DependsOn = []int{prev}
		}
		step.Output = orig.Output + "（第 " + strconv.Itoa(b) + "/" + strconv.Itoa(batches) + " 批）"
		step.Action = orig.Action + "（第 " + strconv.Itoa(b) + "/" + strconv.Itoa(batches) + " 批）"
		out = append(out, step)
		prev = len(out) - 1
	}
	out = append(out, steps[idx+1:]...)
	return out
}

func containsCap(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

// maxDocRunes 是文档参与规划的上限（**UNVALIDATED**；超限截断摘要并标降级）。
const maxDocRunes = 200000

// docSummary 是文档的**摘要**（结构 + 关键词），不是全文。
//
// 纪律：**不许把整个文档塞进提示词**（L3：只注入相关片段）；
// 也**不许只换个说法说"内容已分析"**（那就是造假）——下面每个数都从内容算出来。
type docSummary struct {
	Read                bool
	Lines, Items, TODOs int
	Keywords, hit       []string
	Note                string
	TooLarge            bool
}

func (d docSummary) Describe() string {
	return "document: " + strconv.Itoa(d.Lines) + " 行 / " + strconv.Itoa(d.Items) +
		" 条目 / " + strconv.Itoa(d.TODOs) + " 个 TODO"
}

// Entities 返回可作为"活跃实体"的文档实体（有界，取前 8 个关键词）。
func (d docSummary) Entities() []string {
	if len(d.Keywords) > 8 {
		return d.Keywords[:8]
	}
	return d.Keywords
}

// HitKeywords 返回任务里出现过的文档关键词（任务↔文档关联可见）。
func (d docSummary) HitKeywords(task string) []string {
	var out []string
	for _, kw := range d.Keywords {
		if strings.Contains(task, kw) {
			out = append(out, kw)
		}
	}
	return out
}

// summarizeDocument 从文档内容算出**结构 + 关键词**（有界、确定性）。
func summarizeDocument(doc string) docSummary {
	var d docSummary
	if strings.TrimSpace(doc) == "" {
		d.Note = "document 为空（未参与规划）"
		return d
	}
	runes := []rune(doc)
	if len(runes) > maxDocRunes {
		d.TooLarge = true
		doc = string(runes[:maxDocRunes])
	}
	d.Read = true
	seen := map[string]bool{}
	for _, line := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		d.Lines++
		// ① 条目格式覆盖：列表标记 + TODO/FIXME/XXX 标记行**都算条目**
		if isListItem(t) || isTodoLine(t) {
			d.Items++
		}
		if isTodoLine(t) {
			d.TODOs++
		}
		for _, tok := range tokenize(t) {
			if !seen[tok] {
				seen[tok] = true
				d.Keywords = append(d.Keywords, tok)
			}
		}
	}
	if len(d.Keywords) > 20 {
		d.Keywords = d.Keywords[:20]
	}
	if d.TooLarge {
		d.Note = "文档超限，已按前 " + strconv.Itoa(maxDocRunes) + " 字做摘要"
	}
	return d
}

// isListItem 识别常见列表/标记格式（**每种格式都要有判据**）。
func isListItem(t string) bool {
	for _, p := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	// 数字点/括号：1. / 1) / 1、
	digits := 0
	for digits < len(t) && t[digits] >= '0' && t[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits < len(t) {
		switch t[digits] {
		case '.', ')':
			return true
		}
		if strings.HasPrefix(t[digits:], "、") {
			return true
		}
	}
	return false
}

// isTodoLine 识别 TODO/FIXME/XXX 标记行（带或不带冒号）。
func isTodoLine(t string) bool {
	u := strings.ToUpper(t)
	for _, m := range []string{"TODO", "FIXME", "XXX"} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// tokenize 抽出可比较的词（中日韩 ≥2 字、拉丁 ≥3 字），忽略标点与空白。
func tokenize(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) >= 2 {
			out = append(out, string(cur))
		}
		cur = nil
	}
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF: // 汉字
			cur = append(cur, r)
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return out
}
