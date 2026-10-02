// Package risk 是确认策略引擎（设计 v2 §4）：用三信号（可逆性/影响面/置信度）
// 把一个意图裁决为 auto|light|strong|human 四级确认策略。
// 影响面走机械信号（文件被引用次数/热度/测试覆盖），不靠模型自评；
// 不可逆是硬规则，永远人工确认，且不可被三元组缓存学习掉。
// 本包只依赖标准库 + contract。
package risk

// 【伪代码逻辑层】（评审关卡产物；判断语义的权威定义在设计 v2 §4 / VSL 判断语义层，
//  本层只描述单模块控制流/分支/异常路径，不重复规则本体——规则语义标注"搬 VSL"。）
//
// 模块职责：输入=意图（硬事实：是否可逆）+ 机械信号 ImpactInput；
//   输出=Decision.Level（auto|light|strong|human）+ 人话 Reason。
//
// StaticImpact(in ImpactInput) -> string（搬 VSL：影响面阈值定义）：
//   base = small
//   if in.RefCount >= 8:            base = high
//   else if in.RefCount >= 3:       base = medium
//   else if in.Heat >= 10:          base = medium   // 热度高但引用少，仍按中影响处理（v2 细化）
//   return base
//
// Evaluate(intent, imp) -> Decision：
//   1. 不可逆硬门禁（优先级最高，无视其余信号；搬 VSL：不可逆意图清单）：
//      if intent == COMMIT or intent == DEPLOY:            return human
//      if intent == EDIT and intent.Params["action"]=="delete": return human
//      if intent.Space == "vault-creds":                   return human  // 凭证库高敏
//   2. 可逆性取值（搬 VSL）：
//      reversible = true（默认）；intent.Risk 非空时取 intent.Risk.Reversible
//      if reversible == false: return human   // 基线已标不可逆，同样硬门禁
//   3. 影响面 = StaticImpact(imp)（机械算，不读 intent.Risk.Impact 的模型自评）
//   4. 置信度 = intent.Risk.Confidence 非 0 用之，否则 intent.Confidence；阈值 0.7
//   5. 决策矩阵（搬 VSL 设计 v2 §4 表）：
//      switch impact:
//        case small:
//           if conf >= 0.7: level=auto  reason="可逆+小+高：自动执行+标待抽查"
//           else:           level=auto  reason="可逆+小+低：自动执行+待抽查+回执高亮"
//        case medium: level=light reason="可逆+中：展示 diff 摘要→轻确认"
//        case high:   level=strong reason="可逆+高：diff 预览+影响分析→强确认"
//   异常：intent.Risk 为 nil → 可逆=true、置信度回落 intent.Confidence，不报错（薄降级）。
//
// Guard（确认疲劳防护，搬 VSL 设计 v2 §4）：
//   ShouldDowngrade(path) -> bool：
//      // 唯一入口：既记录本次强确认路径，又裁决是否降级。
//      if path == g.lastPath: g.counts[path]++ else g.counts[path]=1
//      g.lastPath = path
//      return g.counts[path] > 3   // 同路径连续 >3 次强确认 → 降为"汇总待复核"

import (
	"voicesign-harness/contract"
)

// ImpactInput 是影响面机械信号（不靠模型自评；设计 v2 §4）。
type ImpactInput struct {
	RefCount int  // 文件被引用次数
	HasTest  bool // 是否有测试覆盖
	Heat     int  // 历史改动热度（近 N 次改动计数）
}

// Signals 是参与裁决的三信号归一形态（可逆性硬规则 + 机械影响面 + 置信度软信号）。
type Signals struct {
	Revertible bool
	Impact     string
	Confidence float64
}

// Decision 是裁决结果：Level ∈ auto|light|strong|human，Reason 人话解释（回执/审计用）。
type Decision struct {
	Level  string
	Reason string
}

// confHigh 是"高置信"阈值（软信号，只决定 auto 内是否高亮，不升级级别）。
const confHigh = 0.7

// StaticImpact 由机械信号算影响面：RefCount≥8→high；≥3→medium；热度≥10 补升到 medium；否则 small。
func StaticImpact(in ImpactInput) string {
	switch {
	case in.RefCount >= 8:
		return contract.ImpactHigh
	case in.RefCount >= 3:
		return contract.ImpactMedium
	case in.Heat >= 10:
		return contract.ImpactMedium
	default:
		return contract.ImpactSmall
	}
}

// irreversible 报告该意图是否落入不可逆硬门禁（COMMIT/DEPLOY/删除/凭证库）。
// 搬 VSL：不可逆意图清单（COMMIT/DEPLOY/DELETE/外发/凭证），不可被学习掉。
func irreversible(it *contract.Intent) bool {
	switch it.Intent {
	case contract.IntentCommit, contract.IntentDeploy:
		return true
	case contract.IntentEdit:
		if it.Params != nil && it.Params["action"] == "delete" {
			return true
		}
	}
	if it.Space == "vault-creds" {
		return true
	}
	return false
}

// Evaluate 按三信号决策矩阵裁决确认等级。不可逆 → human（硬门禁，先于一切）。
func Evaluate(it contract.Intent, imp ImpactInput) Decision {
	// 1. 不可逆硬门禁
	if irreversible(&it) {
		return Decision{Level: contract.ConfirmHuman, Reason: "不可逆动作（提交/部署/删除/外发/凭证）→ 永远人工确认"}
	}
	// 2. 可逆性（基线标不可逆同样硬门禁）
	reversible := true
	conf := it.Confidence
	if it.Risk != nil {
		reversible = it.Risk.Reversible
		if it.Risk.Confidence != 0 {
			conf = it.Risk.Confidence
		}
	}
	if !reversible {
		return Decision{Level: contract.ConfirmHuman, Reason: "意图基线标不可逆 → 永远人工确认"}
	}
	// 3. 机械影响面 + 5. 决策矩阵
	switch StaticImpact(imp) {
	case contract.ImpactMedium:
		return Decision{Level: contract.ConfirmLight, Reason: "可逆+中影响：展示 diff 摘要 → 轻确认"}
	case contract.ImpactHigh:
		return Decision{Level: contract.ConfirmStrong, Reason: "可逆+高影响：diff 预览+影响分析 → 强确认"}
	default: // small
		if conf >= confHigh {
			return Decision{Level: contract.ConfirmAuto, Reason: "可逆+小+高置信：自动执行 + 标待抽查"}
		}
		return Decision{Level: contract.ConfirmAuto, Reason: "可逆+小+低置信：自动执行 + 待抽查 + 回执高亮"}
	}
}

// Guard 是确认疲劳防护器：统计同路径连续强确认次数，超限即降级为汇总待复核。
type Guard struct {
	lastPath string
	counts   map[string]int
}

// NewGuard 构造空 Guard。
func NewGuard() *Guard {
	return &Guard{counts: map[string]int{}}
}

// ShouldDowngrade 记录一次对 path 的强确认，并裁决同路径连续强确认是否已 >3 次。
// 返回 true 表示本次应降为"汇总待复核"，不再实时打断。
func (g *Guard) ShouldDowngrade(path string) bool {
	if path == "" {
		path = "<empty>"
	}
	if path == g.lastPath {
		g.counts[path]++
	} else {
		g.counts[path] = 1
	}
	g.lastPath = path
	return g.counts[path] > 3
}
