// report.go -- use  answer"  split tobothas hasuse"(VHS-FASTSLOW-001 §3     pt). 
//
//   origthen: **kindbase  then "no   "**,     kindbase close . 
package route

import (
	"fmt"
	"strings"
)

// MinSamplesForVerdict isgiveoutclose  need   kindbase (**UNVALIDATED**:      value). 
const MinSamplesForVerdict = 10

// Report  out    pt   (  read). 
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

	// ①   split 
	if s.Total < MinSamplesForVerdict {
		fmt.Fprintf(&b, "考察点① L0 比例: 样本不足（%d <%d），无法判定\n", s.Total, MinSamplesForVerdict)
	} else if s.L0Share < 0.5 {
		fmt.Fprintf(&b, "考察点① L0 比例: %.2f 偏低 —— 什么都下沉到模型（太慢）\n", s.L0Share)
	} else {
		fmt.Fprintf(&b, "考察点① L0 比例: %.2f 正常\n", s.L0Share)
	}
	// ②   rate
	if s.Total < MinSamplesForVerdict {
		fmt.Fprintf(&b, "考察点② 升级率: 样本不足（%d <%d），无法判定\n", s.Total, MinSamplesForVerdict)
	} else if s.EscalationRate > 0.5 {
		fmt.Fprintf(&b, "考察点② 升级率: %.2f 偏高 —— 快模型（L0.5）没留住\n", s.EscalationRate)
	} else {
		fmt.Fprintf(&b, "考察点② 升级率: %.2f 正常\n", s.EscalationRate)
	}
	// ③   afteris change  -- onlyuse**sametime  beforeafter  **  objto 
	fmt.Fprintf(&b, "考察点③ 可比样本: %d（缺前后质量字段的条目: %d —— 按 unknown 计，不当 0）\n",
		s.ComparableQuality, s.UnknownQuality)
	switch {
	case s.ComparableQuality < MinSamplesForVerdict:
		fmt.Fprintf(&b, "考察点③ 升级后是否变好: 样本不足（可比 %d < %d），无法判定\n",
			s.ComparableQuality, MinSamplesForVerdict)
	case s.Improved > s.Worsened:
		fmt.Fprintf(&b, "考察点③ 升级后是否变好: 改善 %d vs 变差 %d ⇒ 升级有正面效果\n", s.Improved, s.Worsened)
	case s.Improved < s.Worsened:
		fmt.Fprintf(&b, "考察点③ 升级后是否变好: 改善 %d vs 变差 %d ⇒ 不改善（白升级，升级规则需重审）\n", s.Improved, s.Worsened)
	default:
		fmt.Fprintf(&b, "考察点③ 升级后是否变好: 改善 %d vs 变差 %d ⇒ 无差异\n", s.Improved, s.Worsened)
	}
	return b.String(), nil
}
