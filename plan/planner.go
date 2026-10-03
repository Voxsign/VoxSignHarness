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
	Tool      string            `json:"tool"`             // 必须是 Manifest.Tools 里的能力名
	Caps      []string          `json:"caps"`             // 用到的 cap（如 git 的 commit）
	Params    map[string]string `json:"params"`           // 明确参数（PL-3：不得空泛）
	Action    string            `json:"action"`           // 可执行动作（不得是"分析一下"这类空话）
	Output    string            `json:"output"`           // 产出物（PL-3）
	Why       string            `json:"why,omitempty"`    // PM-5：这一步的依据（不得是黑盒）
	Domain    string            `json:"domain,omitempty"` // PM-4：目标域（本机按 AllowedSpaces 复核）
	DependsOn []int             `json:"depends_on,omitempty"`
}

// ConsideredItem 是"这次规划**考虑了哪个要素**"（P2 先决条件：没有观测面，W_* 测不出来）。
type ConsideredItem struct {
	Element  string `json:"element"`
	Source   string `json:"source"`   // WM-3：每条必须有来源
	Inferred bool   `json:"inferred"` // 推断来的（而非事实）
}

// WMStats 是工作记忆的可测指标（WORKMEM-001 §3）。
type WMStats struct {
	Max       int      `json:"w_max"`                  // 一次规划同时引用的要素上界（容量）
	Used      int      `json:"w_used"`                 // 实际用到
	Drop      int      `json:"w_drop"`                 // 因握不住被丢弃的要素数
	DropTrace []string `json:"w_drop_trace,omitempty"` // WM-2：丢弃必须留痕
	ChainLen  int      `json:"chain_len"`              // 计划能排到第几步
	// WMCAP 留痕（VHS-WMCAP-001 C1/C2/WMC-5）：容量变化必须可解释。
	Capacity    int     `json:"capacity,omitempty"`
	DemandFloor int     `json:"demand_floor,omitempty"`
	Familiarity float64 `json:"familiarity,omitempty"`
	Capped      bool    `json:"capped,omitempty"`
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

	Considered     []ConsideredItem `json:"considered,omitempty"`      // 观测面（P2）
	WM             WMStats          `json:"wm,omitempty"`              // 可测的 W（P2）
	Source         string           `json:"source,omitempty"`          // rule | model | readonly-fallback（PM-5 可解释）
	Degraded       bool             `json:"degraded,omitempty"`        // PM-2：模型失败已降级
	DegradedReason string           `json:"degraded_reason,omitempty"` // 降级原因（不得静默）
}

// Planner 是规划器接口。
type Planner interface {
	Plan(goal string, m Manifest) (Plan, error)
}

// LocalPlanner 是规则式规划器：只用 Manifest 内的能力，离线可复现。
type LocalPlanner struct{}

// 目标关键词 → 必需能力。缺失即"做不到"（不猜、不编）。
var requirementRules = []struct {
	words []string
	tool  string
	auth  bool   // true = 不可逆/生产影响 ⇒ 授权在前（U1：授权才是本质）
	kind  string // 网关请求类型：工具 | 模型（U3/C3）
}{
	{[]string{"部署", "上线", "deploy"}, "deploy", true, "工具"},
	{[]string{"发布到生产", "生产发布"}, "release", true, "工具"},
	{[]string{"删除", "删掉", "移除文件", "rm "}, "delete", true, "工具"},
	{[]string{"推到", "推送", "push", "远端分支"}, "push", false, "工具"},
	{[]string{"上架", "app store", "软件商店", "应用商店"}, "appstore", false, "工具"},
	{[]string{"邮件", "email", "发信"}, "email", false, "工具"},
	{[]string{"上传", "upload"}, "upload", false, "工具"},
	{[]string{"判断一下", "评估一下", "帮我判断", "帮我评估"}, "model-judgment", false, "模型"},
}

// bypassWords 是"要求跳过确认"的口语形态（F3：不得无声执行被保护动作）。
var bypassWords = []string{"不用确认", "不用我确认", "无须确认", "别问", "不用问", "直接提交", "自行决定", "不用请示"}

// protectedCapsIn 找出目标里涉及的被保护动作（不可逆/需授权）。
func protectedCapsIn(goal string) []string {
	var out []string
	if containsAny(goal, "提交", "commit") {
		out = append(out, "commit")
	}
	if containsAny(goal, "部署", "上线", "发布", "deploy") {
		out = append(out, "deploy")
	}
	if containsAny(goal, "删除", "删掉") {
		out = append(out, "delete")
	}
	return out
}

// selfServiceWords 是"只读自服务"目标形态（SC-1：必须有模板）。
var selfServiceWords = []string{"读一下", "看一下", "看看", "列出", "数一下", "几个", "跑一下", "查一下", "告诉我"}

// readonlyProbeSteps 返回**只读探查**步骤（F1 裁决：拒绝承诺，但可以去看看）。
// 只允许 file.read / search.text / test.run / git.status；Action 与 Why 不得声称"完成/达成"。
func readonlyProbeSteps() []Step {
	return []Step{
		{
			Tool: "search", Caps: []string{"text"},
			Params: map[string]string{"pattern": ".", "path": "."},
			Action: "查看相关文件与上下文（只读探查）", Output: "匹配结果（供判断）",
			Why: "收集信息以便判断该目标是否可做（只读探查）",
		},
		{
			Tool: "git", Caps: []string{"status"},
			Params: map[string]string{"args": "status"},
			Action: "查看工作区状态（只读探查）", Output: "git status 输出",
			Why: "收集当前仓库状态供后续判断（只读探查）",
		},
	}
}

// PlanWithMemory 按给定容量重规划（WM-1：容量越大，能握住的实体越多）。
// goal 形如 "目标|实体1|实体2|…"：'|' 之后是被注入的活跃实体。
func (LocalPlanner) PlanWithMemory(goal string, m Manifest, capacity int) (Plan, error) {
	parts := strings.Split(goal, "|")
	w := &WorkingMemory{Capacity: capacity}
	for _, e := range parts[1:] {
		w.Remember("working_set", BoardItem{Element: strings.TrimSpace(e), Source: "caller"})
	}
	return LocalPlanner{}.planWithWorkingMemory(parts[0], m, w)
}

// PlanWithWorkingMemory 是"带工作记忆"的规划入口（WM-5 的观测口径）：
// 有记忆 ⇒ 能消解指代、能排链；清空 ⇒ 只能回问。
func (LocalPlanner) PlanWithWorkingMemory(goal string, m Manifest, w *WorkingMemory) (Plan, error) {
	return LocalPlanner{}.planWithWorkingMemory(goal, m, w)
}

func (LocalPlanner) planWithWorkingMemory(goal string, m Manifest, w *WorkingMemory) (Plan, error) {
	if w == nil || len(w.BoundedWorkingSet()) == 0 {
		cons, wm := observe(goal, m, nil, nil)
		return Plan{
			Goal: goal, Source: "rule", Refused: true,
			Missing:    []string{"人：目标含指代或需要上下文，但工作记忆为空，请明确是哪个对象"},
			Reason:     "工作记忆为空：无法消解指代，不脑补",
			Considered: cons, WM: wm,
		}, nil
	}
	// 约束板有**否决权**：命中约束即拒绝（不得绕过）。
	for _, c := range w.Constraints {
		if strings.Contains(goal, c.Element) {
			cons, wm := observe(goal, m, nil, nil)
			return Plan{Goal: goal, Source: "rule", Refused: true,
				Missing: []string{"人：该动作命中约束「" + c.Element + "」（source=" + c.Source + "），须人工裁决"},
				Reason:  "约束板否决", Considered: cons, WM: wm}, nil
		}
	}
	// 待决板非空 ⇒ 先回问（不猜）。
	if len(w.OpenItems) > 0 {
		cons, wm := observe(goal, m, nil, nil)
		return Plan{Goal: goal, Source: "rule", Refused: true,
			Missing:    []string{"人：待决项未解决 —— " + w.OpenItems[0].Element},
			Reason:     "待决板非空，先问再动",
			Considered: cons, WM: wm}, nil
	}
	// 需求下限 = 目标里**显式提到**的实体数（一个都不提 ⇒ 1）；
	// 供给 = 活跃实体板。两者分开，容量才有意义（否则永远够用、公式空转）。
	if w.DemandFloor <= 0 {
		w.DemandFloor = 1
		for _, it := range w.WorkingSet {
			if strings.Contains(goal, it.Element) {
				w.DemandFloor++
			}
		}
	}
	// 活跃实体板驱动链长：每个被握住的实体 ⇒ 读 + 改，末尾提交。
	var steps []Step
	for range w.BoundedWorkingSet() {
		steps = append(steps,
			Step{Tool: "file", Caps: []string{"read"}, Params: map[string]string{"path": "<实体>"}, Action: "读取实体当前内容", Output: "内容"},
			Step{Tool: "file", Caps: []string{"write"}, DependsOn: []int{len(steps) - 1}, Params: map[string]string{"path": "<实体>", "content": "<修改后>"}, Action: "写入修改", Output: "修改后的文件"},
		)
	}
	steps = append(steps, Step{Tool: "git", Caps: []string{"commit"}, DependsOn: []int{len(steps) - 1},
		Params: map[string]string{"args": "commit", "message": "<说明>"}, Action: "提交改动", Output: "commit hash"})
	if missing := missingTools(m, steps); len(missing) > 0 {
		cons, wm := observe(goal, m, steps, nil)
		return Plan{Goal: goal, Source: "rule", Refused: true,
			Missing: []string{"网关：" + strings.Join(missing, ",") + " 不在能力清单内"}, Considered: cons, WM: wm}, nil
	}
	cons, wm := observe(goal, m, steps, nil)
	ct := w.CapacityFor()
	wm.Capacity, wm.DemandFloor, wm.Familiarity, wm.Capped = ct.Capacity, ct.DemandFloor, ct.Familiarity, ct.Capped
	plan := Plan{Goal: goal, Source: "rule", Steps: steps, Considered: cons, WM: wm}
	plan.WM.Drop = len(w.DropTrace)
	plan.WM.DropTrace = append([]string(nil), w.DropTrace...)
	if plan.WM.Drop > 0 {
		plan.Degraded = true
		plan.DegradedReason = "工作记忆容量不足，已丢弃 " + strings.Join(w.DropTrace, ",") + "（留痕，非静默）"
	}
	return plan, nil
}

// wmCapacity 是一次规划"握得住"的要素数上界（WORKMEM-001：容量有界，丢了要留痕）。
const wmCapacity = 8

// observe 记录本次规划考虑了哪些要素（WM-2/WM-3），并算出可测的 W。
func observe(goal string, m Manifest, steps []Step, gaps []string) ([]ConsideredItem, WMStats) {
	var items []ConsideredItem
	for _, r := range requirementRules {
		if containsAny(goal, r.words...) {
			for _, w := range r.words {
				if strings.Contains(goal, strings.ToLower(w)) {
					items = append(items, ConsideredItem{Element: "goal:" + w, Source: "plan/planner.go:requirementRules", Inferred: false})
					break
				}
			}
		}
	}
	for _, c := range m.Tools {
		if len(steps) > 0 {
			for _, s := range steps {
				if s.Tool == c.Name {
					items = append(items, ConsideredItem{Element: "capability:" + c.Name, Source: "tools/registry.go", Inferred: false})
					break
				}
			}
		}
	}
	for _, g := range gaps {
		items = append(items, ConsideredItem{Element: "gap:" + g, Source: "plan/planner.go:gapRequirements", Inferred: true})
	}
	wm := WMStats{Max: wmCapacity, ChainLen: len(steps)}
	if len(items) > wmCapacity {
		for _, it := range items[wmCapacity:] {
			wm.DropTrace = append(wm.DropTrace, it.Element) // WM-2：静默丢弃禁止
		}
		items = items[:wmCapacity]
	}
	wm.Used = len(items)
	wm.Drop = len(wm.DropTrace)
	return items, wm
}

// Plan 按规则产出一个确定性计划。
//
// 分类（六类）：reachable / self_service / partial / capability_gap / authorization_gap / 未识别。
// 未识别 ⇒ Refused + **只读探查**（Source=readonly-probe，且不得声称覆盖目标）。
func (LocalPlanner) Plan(goal string, m Manifest) (Plan, error) {
	g := strings.ToLower(strings.TrimSpace(goal))
	if g == "" {
		return Plan{Goal: goal, Source: "rule", Refused: true, Missing: []string{"无外部依赖：目标为空"},
			Reason: "目标为空，不做任何假设"}, nil
	}
	// authorization_gap（F3）：要求绕过确认 + 被保护动作 ⇒ 拒绝并指人。
	if containsAny(g, bypassWords...) {
		if caps := protectedCapsIn(g); len(caps) > 0 {
			return Plan{Goal: goal, Source: "rule", Refused: true,
				Missing: []string{"人：被保护动作（" + strings.Join(caps, ",") + "）不可逆/需授权，必须人工确认；规划器不得自行授权"},
				Reason:  "目标要求绕过确认；拒绝静默执行"}, nil
		}
	}

	gaps := gapRequirements(g, m)
	steps, kind := classify(g)

	switch {
	case len(gaps) > 0 && kind == "":
		// capability_gap：已识别的能力缺口 ⇒ 拒绝 + 指 owner，**不编步骤**
		//（SC-2/PL-2 要求：拒绝时不得给步骤；只读探查仅用于"未识别"目标，见下）。
		cons, wm := observe(goal, m, nil, gaps)
		return Plan{Goal: goal, Source: "rule", Refused: true, Missing: gaps, Considered: cons, WM: wm,
			Reason: "目标需要清单外能力（Manifest 即边界），不编造可执行计划"}, nil
	case len(gaps) > 0 && kind != "":
		// partial：只规划可达前缀，尾巴进 Missing（既非整体照做，也非整体拒绝）
		if missing := missingTools(m, steps); len(missing) > 0 {
			return Plan{Goal: goal, Source: "rule", Refused: true,
				Missing: append(gaps, "网关："+strings.Join(missing, ",")+" 不在能力清单内"), Reason: "可达前缀也需要清单外能力"}, nil
		}
		cons, wm := observe(goal, m, steps, gaps)
		return Plan{Goal: goal, Source: "rule", Steps: steps, Missing: gaps, Considered: cons, WM: wm,
			Degraded: true, DegradedReason: "目标部分不可达：只规划可达前缀，未覆盖全部目标"}, nil
	case kind != "":
		if missing := missingTools(m, steps); len(missing) > 0 {
			return Plan{Goal: goal, Source: "rule", Refused: true,
				Missing: []string{"网关：" + strings.Join(missing, ",") + " 不在能力清单内"}, Reason: "计划需要清单外能力"}, nil
		}
		cons, wm := observe(goal, m, steps, nil)
		return Plan{Goal: goal, Source: "rule", Steps: steps, Considered: cons, WM: wm}, nil
	default:
		// 未识别目标：Refused（拒绝承诺）+ 只读探查（去看看）+ 显式自曝。
		probe := readonlyProbeSteps()
		if missing := missingTools(m, probe); len(missing) > 0 {
			probe = nil
		}
		cons, wm := observe(goal, m, probe, nil)
		return Plan{
			Goal: goal, Refused: true, Steps: probe, Source: "readonly-probe", Considered: cons, WM: wm,
			Missing:  []string{"网关：无法判定该目标所需能力（不在已支持的目标形态内），不编造计划；如确需请提需求"},
			Reason:   "无法判定目标所需能力，不编造计划",
			Degraded: true, DegradedReason: "未识别目标：只做只读探查，不声称覆盖目标",
		}, nil
	}
}

// classify 按目标形态返回模板步骤与类别（"" = 未识别）。
func classify(g string) ([]Step, string) {
	switch {
	case containsAny(g, "todo", "整理", "汇总", "文档"):
		return summarizeSteps(), "reachable"
	case containsAny(g, "改", "修改", "编辑", "替换") && containsAny(g, "提交", "commit"):
		return editThenCommitSteps(), "partial_or_reachable"
	case containsAny(g, selfServiceWords...):
		return selfServiceSteps(g), "self_service"
	default:
		return nil, ""
	}
}

// selfServiceSteps 给出只读自服务模板（SC-1）。
func selfServiceSteps(g string) []Step {
	if containsAny(g, "跑", "测试", "go test") {
		return []Step{{
			Tool: "test", Caps: []string{"run"}, Params: map[string]string{"command": "go test ./..."},
			Action: "运行测试并查看结果（只读）", Output: "测试输出",
			Why: "自服务：读取项目当前测试状态",
		}}
	}
	return []Step{{
		Tool: "search", Caps: []string{"text"}, Params: map[string]string{"pattern": ".", "path": "."},
		Action: "查看相关文件内容（只读）", Output: "匹配结果",
		Why: "自服务：读取项目信息",
	}}
}

// gapRequirements 返回目标的缺口条目（C1–C4）：
//   - 授权类 `人：`；能力类 `网关：`（并标明网关请求类型：工具/模型）；
//   - 同一目标可同时有两类缺口，**授权在前**（C2）；
//   - 不可逆/生产影响的动作必须有人工确认指向（C4）。
func gapRequirements(goal string, m Manifest) []string {
	have := map[string]bool{}
	for _, c := range m.Tools {
		have[c.Name] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, r := range requirementRules {
		if !containsAny(goal, r.words...) || seen[r.tool] {
			continue
		}
		seen[r.tool] = true
		if r.auth {
			out = append(out, "人："+r.tool+" 属不可逆/生产影响动作，须人工授权（授权是前置）")
		}
		if !have[r.tool] {
			out = append(out, "网关：且当前无 "+r.tool+" 能力（请求类型="+r.kind+"；域别名不是工具契约）")
		}
	}
	return out
}

// unmetRequirement 报告目标是否要求清单里不具备的能力（保留给模型式复核使用）。
func unmetRequirement(goal string, m Manifest) ([]string, bool) {
	gaps := gapRequirements(goal, m)
	return gaps, len(gaps) == 0
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
