// risk.go —— 工具**风险分级**与「最小风险工具优先」（PM-6）。
//
// 动因（Lead 实测 2026-10-03）：强模型对一个"搜索 TODO"的任务选了 `run`（exec/high）
// 而不是清单里现成的 `search` —— **能调用 ≠ 产出好**；模型天然倾向"挑重的"（看起来更能干，其实更危险）。
//
// 这是**本机硬复核**，不是提示词建议：提示词是软的，复核是硬的。
package plan

import "strings"

// ToolRisk 是风险等级（数值越大越危险）。
type ToolRisk int

const (
	RiskUnknown ToolRisk = iota
	RiskLow              // 只读：search / read / note
	RiskMedium           // 写入本域：file(write) / git(commit 之外的写)
	RiskHigh             // 执行 / 不可逆：run / test / git.commit / deploy
)

// toolRiskTable 是**显式**风险表（不靠"看着像"）。新工具必须在此登记。
var toolRiskTable = map[string]ToolRisk{
	"search": RiskLow,
	"read":   RiskLow,
	"note":   RiskLow,
	"query":  RiskLow,
	"ask":    RiskLow,
	"file":   RiskMedium,
	"git":    RiskMedium,
	"test":   RiskMedium, // 登记表：test=low/medium（跑测试，边界内）；run=high（任意命令）
	"run":    RiskHigh,
}

// ToolRiskOf 返回工具风险；未登记 ⇒ RiskUnknown。
func ToolRiskOf(tool string) ToolRisk {
	if r, ok := toolRiskTable[tool]; ok {
		return r
	}
	return RiskUnknown
}

// searchableWords 是"这其实是查找类任务"的信号词。
var searchableWords = []string{"搜索", "查找", "找到", "定位", "列出", "统计", "整理", "汇总", "所有"}

// needsExecWords 是"确实需要执行命令"的信号词（防"一律禁 run"的过度保守）。
var needsExecWords = []string{"执行", "运行命令", "跑命令", "curl", "安装", "编译", "构建命令"}

// lowerRiskAlternative 若存在**更低风险且能覆盖同一意图**的工具，返回它。
// 只认登记在 risk 表里的工具，避免"猜替代品"。
func lowerRiskAlternative(chosen string, chosenRisk ToolRisk, m Manifest) (string, bool) {
	if chosenRisk < RiskHigh {
		return "", false
	}
	best := ""
	bestRisk := chosenRisk
	for _, c := range m.Tools {
		if c.Name == chosen {
			continue
		}
		r := ToolRiskOf(c.Name)
		if r == RiskUnknown || r >= chosenRisk {
			continue
		}
		if r < bestRisk {
			best, bestRisk = c.Name, r
		}
	}
	return best, best != ""
}

// reviewMinRisk 是 PM-6：查找类任务**不得**用高风险工具，若清单里有更低风险的替代。
// 返回违规说明（空 = 通过）。
func reviewMinRisk(goal string, steps []Step, m Manifest) []string {
	if containsAny(strings.ToLower(goal), needsExecWords...) {
		return nil // 任务确实需要执行 ⇒ 允许高风险工具（防过度保守）
	}
	if !containsAny(goal, searchableWords...) {
		return nil
	}
	var bad []string
	for i, s := range steps {
		r := ToolRiskOf(s.Tool)
		if alt, ok := lowerRiskAlternative(s.Tool, r, m); ok {
			bad = append(bad, fmt_risk(i, s.Tool, alt))
		}
	}
	return bad
}

func fmt_risk(i int, chosen, alt string) string {
	return "第 " + itoa(i) + " 步选了高风险工具 " + chosen + "，清单内存在更低风险替代 " + alt +
		"（PM-6：最小风险工具优先）"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
