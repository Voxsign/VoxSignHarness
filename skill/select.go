// Package skill —— 技能层：白名单筛选与"被过滤项显式记录"（SK-4 口径）。
//
// 口径（Lead 2026-10-03）：**白名单 + state != deprecated**；
// **被过滤项必须显式记录**（不许静默丢弃）；**不删服务侧数据**。
package skill

// Skill 是技能元数据（来自 /api/skill/skills）。
type Skill struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	State   string `json:"state"` // active | deprecated | ...
	Kind    string `json:"kind"`  // 例如 research ⇒ 走远端
}

// Filtered 记录一个被过滤掉的技能**及其原因**（显式，不许静默）。
type Filtered struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Selection 是筛选结果。
type Selection struct {
	Included []Skill    `json:"included"`
	Filtered []Filtered `json:"filtered"`
}

// DefaultWhitelist 是**代码内登记**的内化白名单（SK-10：可审计，不由调用方随手传）。
//
// 依据 Lead 2026-10-03 的内化判断（state != deprecated 另由 StateClass 把关）。
var DefaultWhitelist = map[string]bool{
	"ai-native-architecture-design": true,
	"arch-guardian":                 true,
	"arch-review":                   true,
	"deep-research":                 true,
}

// Select 按白名单筛选：不在白名单 ⇒ not_in_whitelist；state=deprecated ⇒ deprecated。
// **守恒**：len(Included)+len(Filtered) == len(all)（没有任何技能被静默丢弃）。
func Select(all []Skill, allow map[string]bool) Selection {
	var sel Selection
	for _, s := range all {
		switch StateClass(s.State) {
		case "offline":
			sel.Filtered = append(sel.Filtered, Filtered{ID: s.ID, Reason: "offline:" + s.State})
		case "unknown":
			// SK-9：**未登记的 state ⇒ 不纳入 + 记录**（对齐"未登记 ⇒ Unknown，不猜"）
			sel.Filtered = append(sel.Filtered, Filtered{ID: s.ID, Reason: "unknown_state:" + s.State})
		case "online":
			if !allow[s.ID] {
				sel.Filtered = append(sel.Filtered, Filtered{ID: s.ID, Reason: "not_in_whitelist"})
				continue
			}
			sel.Included = append(sel.Included, s)
		}
	}
	return sel
}
