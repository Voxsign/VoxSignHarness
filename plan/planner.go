// planner.go —— 规划器的**规格与桩**（VHS-PLAN-001 §4：PL-1..PL-5）。
//
// P1：只定义形状与接口，零算法。生产规划器（P2）会用外部模型，
// 但**判据本身不依赖模型**：PL 判据只对 Plan 的结构做机械检查，
// 因此可以在模型选型（ASR-MODEL-01）尚未确定时先立先红。
package plan

import "errors"

// ErrNotImplemented 表示能力尚未实现（P1 桩；判据先红，不是判据写错）。
var ErrNotImplemented = errors.New("plan: 未实现（P1 判据先于实现，零算法）")

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

// LocalPlanner 是生产规划器。
//
// P1 状态：**桩**——返回 ErrNotImplemented，于是 PL-1..PL-5 全部为红。
// 它必须只使用 Manifest 内的能力；清单外的目标一律拒绝（PL-1/PL-2）。
type LocalPlanner struct{}

// Plan 见 Planner 接口。P1 桩。
func (LocalPlanner) Plan(goal string, m Manifest) (Plan, error) {
	return Plan{}, ErrNotImplemented
}
