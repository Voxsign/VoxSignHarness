// planner.go —— 规划器（VHS-PLAN-001 §4：PL-1..PL-5）。
//
// 本轮实现是**规则式**的：离线、确定性、只用 Manifest 内的真实能力。
// 它不调用模型 —— PL/RV 判据要求可复现，模型式 planner 天生非确定，
// 应作为**增强**另立判据（不混进这 9 条）。
//
// PL-2（不可达必须明确拒绝）是反幻觉的核心：清单里没有的能力 = 做不到。
package plan

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotImplemented 保留给尚未实现的路径（当前规划器已实现，不再返回它）。
var ErrNotImplemented = errors.New("plan: 未实现")

// ErrUnachievable 表示超出能力边界且复规次数用尽 —— 必须如实说"做不到"。
var ErrUnachievable = errors.New("plan: 超出能力边界，做不到")

// Step 是计划中的一步：必须映射到清单内的真实能力，且参数明确。
type Step struct {
	Tool      string            `json:"tool"`   // 必须是 Manifest.Tools 里的能力名
	Caps      []string          `json:"caps"`   // 用到的 cap（如 git 的 commit）
	Params    map[string]string `json:"params"` // 明确参数（PL-3：不得空泛）
	Action    string            `json:"action"` // 可执行动作（不得是"分析一下"这类空话）
	Output    string            `json:"output"` // 产出物（PL-3）
	DependsOn []int             `json:"depends_on,omitempty"`
}

// Plan 是一次规划的结果。
type Plan struct {
	Goal    string   `json:"goal"`
	Steps   []Step   `json:"steps"`
	Refused bool     `json:"refused"`           // PL-2：不可达时必须为 true
	Missing []string `json:"missing,omitempty"` // PL-2：拒绝时缺什么
	Reason  string   `json:"reason,omitempty"`
	Voided  bool     `json:"voided,omitempty"`  // RV-3：被域门禁拒绝 → 计划作废
	Changes []string `json:"changes,omitempty"` // RV-2：复规时必须说明"变了什么"
}

// Planner 是规划器接口。
type Planner interface {
	Plan(goal string, m Manifest) (Plan, error)
}

// LocalPlanner 是规则式规划器：只用 Manifest 内的能力，离线可复现。
type LocalPlanner struct{}

// 目标关键词 → 必需能力。缺失即"做不到"（不猜、不编）。
var requirementRules = []struct {
	words  []string
	tool   string
	reason string
}{
	{[]string{"部署", "上线", "发布", "deploy"}, "deploy", "缺 deploy 工具契约（域别名 deploy 不是可执行能力）"},
	{[]string{"删除", "删掉", "移除文件", "rm "}, "delete", "缺 delete 工具契约（删除不可逆，须人工确认后另行授权）"},
}

// Plan 按规则产出一个确定性计划。
func (LocalPlanner) Plan(goal string, m Manifest) (Plan, error) {
	g := strings.ToLower(strings.TrimSpace(goal))
	if g == "" {
		return Plan{Goal: goal, Refused: true, Missing: []string{"非空目标"}, Reason: "目标为空，不做任何假设"}, nil
	}
	// PL-2：先判可达性 —— Manifest 即边界。
	if missing, ok := unmetRequirement(g, m); !ok {
		return Plan{Goal: goal, Refused: true, Missing: missing, Reason: "目标需要清单外能力；本规划器不编造可执行计划"}, nil
	}

	var steps []Step
	switch {
	case containsAny(g, "todo", "整理", "汇总", "文档"):
		steps = summarizeSteps()
	case containsAny(g, "改", "修改", "编辑", "替换") && containsAny(g, "提交", "commit"):
		steps = editThenCommitSteps()
	default:
		steps = searchSteps() // 搜索类与兜底：只读（默认拒绝高风险动作）
	}
	// 计划里只允许出现清单内的能力。
	if missing := missingTools(m, steps); len(missing) > 0 {
		return Plan{Goal: goal, Refused: true, Missing: missing, Reason: "计划需要清单内不存在的能力，已拒绝"}, nil
	}
	return Plan{Goal: goal, Steps: steps}, nil
}

// unmetRequirement 报告目标是否要求清单里不具备的能力。
func unmetRequirement(goal string, m Manifest) ([]string, bool) {
	have := map[string]bool{}
	for _, c := range m.Tools {
		have[c.Name] = true
	}
	for _, r := range requirementRules {
		if !containsAny(goal, r.words...) {
			continue
		}
		if !have[r.tool] {
			return []string{r.reason}, false
		}
	}
	return nil, true
}

func missingTools(m Manifest, steps []Step) []string {
	have := map[string]bool{}
	for _, c := range m.Tools {
		have[c.Name] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range steps {
		if !have[s.Tool] && !seen[s.Tool] {
			seen[s.Tool] = true
			out = append(out, s.Tool)
		}
	}
	return out
}

func summarizeSteps() []Step {
	return []Step{
		{
			Tool: "search", Caps: []string{"text"},
			Params: map[string]string{"pattern": "TODO", "path": "."},
			Action: "搜索项目内的 TODO 标记", Output: "TODO 列表（文件:行:内容）",
		},
		{
			Tool: "file", Caps: []string{"write"}, DependsOn: []int{0},
			Params: map[string]string{"path": "docs/TODO-汇总.md", "content": "# TODO 汇总（由搜索结果生成）"},
			Action: "把 TODO 列表写入汇总文档", Output: "docs/TODO-汇总.md",
		},
	}
}

func editThenCommitSteps() []Step {
	return []Step{
		{
			Tool: "file", Caps: []string{"read"},
			Params: map[string]string{"path": "<待改文件>"},
			Action: "读取待改文件原文", Output: "待改文件内容",
		},
		{
			Tool: "file", Caps: []string{"write"}, DependsOn: []int{0},
			Params: map[string]string{"path": "<待改文件>", "content": "<修改后内容>"},
			Action: "写入修改后的内容", Output: "修改后的文件",
		},
		{
			Tool: "git", Caps: []string{"commit"}, DependsOn: []int{1},
			Params: map[string]string{"args": "commit", "message": "<提交说明>"},
			Action: "提交改动", Output: "commit hash",
		},
	}
}

func searchSteps() []Step {
	return []Step{{
		Tool: "search", Caps: []string{"text"},
		Params: map[string]string{"pattern": "TODO", "path": "."},
		Action: "搜索匹配文件", Output: "匹配文件列表",
	}}
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 复规（RV-1..RV-4）
// ---------------------------------------------------------------------------

// StepFailure 是一次步骤失败/被拒的描述（RV-1/RV-3 的输入）。
type StepFailure struct {
	StepIndex    int      `json:"step_index"`
	Tool         string   `json:"tool"`
	Caps         []string `json:"caps,omitempty"`
	Reason       string   `json:"reason"`
	DomainDenied bool     `json:"domain_denied,omitempty"` // RV-3：被域门禁拒绝
	DeniedDomain string   `json:"denied_domain,omitempty"`
}

// MaxReplans 是复规次数上限（RV-4：不无限复规）。
const MaxReplans = 3

// Replan 在某步失败后重新规划（规则式、确定性）。
//
//   - attempt 超 MaxReplans → ErrUnachievable（如实说做不到，RV-4）
//   - 域门禁拒绝 → 计划作废、移除该步、绝不换说法绕过（RV-3）
//   - 其它失败 → 移除失败步并插入只读诊断步，Changes 说明变化（RV-1/RV-2）
func Replan(orig Plan, f StepFailure, m Manifest, attempt int) (Plan, error) {
	if attempt > MaxReplans {
		return Plan{}, ErrUnachievable
	}
	out := orig
	if f.DomainDenied {
		out.Voided = true
		out.Changes = []string{fmt.Sprintf(
			"第 %d 步（%s/%v）被域 %q 拒绝：计划作废，不换说法绕过门禁",
			f.StepIndex, f.Tool, f.Caps, f.DeniedDomain)}
		out.Steps = dropStep(orig.Steps, f.StepIndex)
		out.Steps = ensureNonEmpty(out.Steps, m)
		return out, nil
	}
	out.Changes = []string{fmt.Sprintf(
		"第 %d 步（%s）失败：%s → 移除失败步，插入只读诊断步后再决定后续",
		f.StepIndex, f.Tool, f.Reason)}
	out.Steps = dropStep(orig.Steps, f.StepIndex)
	if diag, ok := diagnosticStep(m); ok {
		out.Steps = insertAt(out.Steps, f.StepIndex, diag)
	}
	out.Steps = ensureNonEmpty(out.Steps, m)
	return out, nil
}

func dropStep(steps []Step, idx int) []Step {
	if idx < 0 || idx >= len(steps) {
		return append([]Step(nil), steps...)
	}
	out := make([]Step, 0, len(steps)-1)
	out = append(out, steps[:idx]...)
	out = append(out, steps[idx+1:]...)
	return out
}

func insertAt(steps []Step, idx int, s Step) []Step {
	if idx < 0 {
		idx = 0
	}
	if idx > len(steps) {
		idx = len(steps)
	}
	out := make([]Step, 0, len(steps)+1)
	out = append(out, steps[:idx]...)
	out = append(out, s)
	out = append(out, steps[idx:]...)
	return out
}

// diagnosticStep 选一个**只读**诊断步（优先 git.status，其次 search.text）。
func diagnosticStep(m Manifest) (Step, bool) {
	caps := map[string]map[string]bool{}
	for _, c := range m.Tools {
		cs := map[string]bool{}
		for _, cap := range c.Caps {
			cs[cap] = true
		}
		caps[c.Name] = cs
	}
	if caps["git"]["status"] {
		return Step{Tool: "git", Caps: []string{"status"}, Params: map[string]string{"args": "status"},
			Action: "读取工作区状态以定位失败原因", Output: "git status 输出"}, true
	}
	if caps["search"]["text"] {
		return Step{Tool: "search", Caps: []string{"text"}, Params: map[string]string{"pattern": ".", "path": "."},
			Action: "搜索相关上下文以定位失败原因", Output: "匹配结果"}, true
	}
	return Step{}, false
}

// ensureNonEmpty 保证复规结果不是空计划（空计划无法执行、也无法审计）。
func ensureNonEmpty(steps []Step, m Manifest) []Step {
	if len(steps) > 0 {
		return steps
	}
	if s, ok := diagnosticStep(m); ok {
		return []Step{s}
	}
	return []Step{{
		Tool: "search", Caps: []string{"text"}, Params: map[string]string{"pattern": ".", "path": "."},
		Action: "只读诊断（清单内无更合适能力）", Output: "诊断输出",
	}}
}
