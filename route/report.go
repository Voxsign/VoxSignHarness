// report.go —— 用台账回答"这套分层到底有没有用"（VHS-FASTSLOW-001 §3 三个考察点）。
//
// 诚实原则：**样本不足就说"无法判定"**，不拿几条样本编结论。
package route

import (
	"fmt"
	"strings"
)

// MinSamplesForVerdict 是给出结论所需的最小样本量（**UNVALIDATED**：我定的经验值）。
const MinSamplesForVerdict = 10

// Report 输出三个考察点的判定（人可读）。
func Report(path string) (string, error) {
	s, err := Aggregate(path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "样本量: %d\n", s.Total)
	fmt.Fprintf(&b, "层级分布: L0=%d L0.5=%d L1=%d L2=%d\n",
		s.ByLevel[LevelL0], s.ByLevel[LevelL05], s.ByLevel[LevelL1], s.ByLevel[LevelL2])
	if s.Total == 0 {
		b.WriteString("考察点① L0 比例: 无法判定（台账为空）\n")
		b.WriteString("考察点② 升级率: 无法判定（台账为空）\n")
		b.WriteString("考察点③ 升级后是否变好: 无法判定（台账为空）\n")
		return b.String(), nil
	}
	fmt.Fprintf(&b, "L0 比例: %.2f ; 升级率: %.2f ; 回问率: %.2f ; 降级率: %.2f\n",
		s.L0Share, s.EscalationRate, s.AskedUserRate, s.DegradedRate)
	fmt.Fprintf(&b, "kind 落空次数: %d ; 路由多命中/未命中次数: %d\n", s.KindFallbacks, s.RouteAmbiguous)

	// ① 层级分布
	if s.Total < MinSamplesForVerdict {
		fmt.Fprintf(&b, "考察点① L0 比例: 样本不足（%d <%d），无法判定\n", s.Total, MinSamplesForVerdict)
	} else if s.L0Share < 0.5 {
		fmt.Fprintf(&b, "考察点① L0 比例: %.2f 偏低 —— 什么都下沉到模型（太慢）\n", s.L0Share)
	} else {
		fmt.Fprintf(&b, "考察点① L0 比例: %.2f 正常\n", s.L0Share)
	}
	// ② 升级率
	if s.Total < MinSamplesForVerdict {
		fmt.Fprintf(&b, "考察点② 升级率: 样本不足（%d <%d），无法判定\n", s.Total, MinSamplesForVerdict)
	} else if s.EscalationRate > 0.5 {
		fmt.Fprintf(&b, "考察点② 升级率: %.2f 偏高 —— 快模型（L0.5）没留住\n", s.EscalationRate)
	} else {
		fmt.Fprintf(&b, "考察点② 升级率: %.2f 正常\n", s.EscalationRate)
	}
	// ③ 升级后是否变好 —— 需要"同一任务升级前后结果对比"的字段
	b.WriteString("考察点③ 升级后是否变好: **无法判定** —— 台账当前**没有**记录" +
		"「同一 task 的升级前后结果/质量」，因此无从对比。\n" +
		"  需要的字段（尚缺）：task_id（同一任务串联）、升级前结果质量、升级后结果质量、outcome_after。\n" +
		"  在此之前，任何\"升级有效\"的说法都只是推断。\n")
	return b.String(), nil
}
