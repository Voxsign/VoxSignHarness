// risk.go --   **risk grading**and"  risk   first"(PM-6). 
//
//  because(Lead    2026-10-03):   typeto  "   TODO" task  `run`(exec/high)
// but islist nowbecome  `search` -- ** calluse != produceout **;  typedayhowever to" heavy "( raise change  , its change risk). 
//
//  is**base    **,  is showword  :  showwordis  ,   is  . 
package plan

import "strings"

// ToolRisk isrisketc (numvalue    risk). 
type ToolRisk int

const (
	RiskUnknown ToolRisk = iota
	RiskLow              // read-only: search / read / note
	RiskMedium           // writebasedomain: file(write) / git(commit ofout write)
	RiskHigh             //    /  reversible: run / test / git.commit / deploy
)

// toolRiskTable is** form**risktable(  " ing "). new        . 
var toolRiskTable = map[string]ToolRisk{
	"search": RiskLow,
	"read":   RiskLow,
	"note":   RiskLow,
	"query":  RiskLow,
	"ask":    RiskLow,
	"file":   RiskMedium,
	"git":    RiskMedium,
	"test":   RiskMedium, //   table: test=low/medium(   ,  boundaryin); run=high(    )
	"run":    RiskHigh,
}

// ToolRiskOf returnback  risk;     ⇒ RiskUnknown. 
func ToolRiskOf(tool string) ToolRisk {
	if r, ok := toolRiskTable[tool]; ok {
		return r
	}
	return RiskUnknown
}

// searchableWords is" its is  classtask" signalword. 
var searchableWords = []string{"搜索", "查找", "找到", "定位", "列出", "统计", "整理", "汇总", "所有"}

// needsExecWords is"  needneed    " signalword(prevent"  forbid run" ed keep ). 
var needsExecWords = []string{"执行", "运行命令", "跑命令", "curl", "安装", "编译", "构建命令"}

// lowerRiskAlternative ifstore **change riskand overwritesame intent**   , returnback . 
// only     risk table    ,   "    ". 
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

// reviewMinRisk is PM-6:   classtask**  **use risk  , iflist haschange risk   . 
// returnback rule  (empty =  ed). 
func reviewMinRisk(goal string, steps []Step, m Manifest) []string {
	if containsAny(strings.ToLower(goal), needsExecWords...) {
		return nil // task  needneed   ⇒  allow risk  (prevented keep )
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
