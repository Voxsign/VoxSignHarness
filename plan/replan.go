// replan.go —— 复规的**规格与桩**（VHS-PLAN-001 §5：RV-1..RV-4）。
//
// 「规划完了之后再怎么改」：失败 → 复规（不是盲重试）；
// 被域门禁拒绝 → 计划作废（不得换个说法绕过）；复规有次数上限，超限如实报做不到。
//
// P1：只定义形状与桩，零算法 —— RV-1..RV-4 应当全部为红。
package plan

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

// Replan 在某步失败后重新规划；attempt 从 1 开始，超 MaxReplans 必须返回 ErrUnachievable。
//
// P1 状态：**桩**——返回 ErrNotImplemented，于是 RV-1..RV-4 全部为红。
func Replan(orig Plan, f StepFailure, m Manifest, attempt int) (Plan, error) {
	return Plan{}, ErrNotImplemented
}
