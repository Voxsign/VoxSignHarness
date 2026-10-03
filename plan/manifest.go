// Package plan —— 自规划能力（VHS-PLAN-001）的**规格与判据**。
//
// P1（判据先于实现）：本文件只定义**能力清单的形状**与导出函数的签名/桩，
// 不含任何导出算法。真值来源（已由探针核实可机械导出）：
//
//	工具  ← tools.LoadContracts(dir).All()   （6 契约：file/git/run/search/test/verify）
//	意图  ← contract 的 Intent* 常量 × 生产代码真实引用（live 17；FILE_WRITE 为 dormant）
//	域    ← space.Load(dir).List()/Get()      （6 内置域模板）
//
// 关键安全取向（探针发现）：`space.Manifest.Tools` 是**另一套域级别名词表**
// （deploy/http/read/query/ask/note/file-append 等 11 处在契约注册表里并不存在），
// 因此能力清单的**工具真值只取契约注册表**；域别名单列 `Aliases` 且带 unresolved 语义，
// **不得**混进 Tools，否则 SK-1 会虚报（把不存在的 deploy 说成能力）。
package plan

import (
	"voicesign-harness/space"
	"voicesign-harness/tools"
)

// Capability 是一条**可执行**能力（来自工具契约注册表）。
type Capability struct {
	Name          string            `json:"name"`
	Caps          []string          `json:"caps"`
	Risk          map[string]string `json:"risk"` // cap → none|low|medium|high|irreversible
	AllowedSpaces []string          `json:"allowed_spaces"`
	SideEffects   []string          `json:"side_effects"`
	NeedsConfirm  map[string]bool   `json:"needs_confirm"` // SK-3：高风险/不可逆 cap 必须为 true
	Source        string            `json:"source"`        // 来源可审计（SK-4）
}

// IntentCapability 是一条意图能力。
type IntentCapability struct {
	Value   string `json:"value"`
	Live    bool   `json:"live"`    // 生产代码真的引用/产出
	Dormant bool   `json:"dormant"` // 只有声明、没有生产引用（不得当可执行能力）
	Source  string `json:"source"`
}

// DomainCapability 是一个域（space），Aliases 是域级名词表（非契约名，unresolved）。
type DomainCapability struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	RiskDefault string   `json:"risk_default"`
	Read        bool     `json:"read"`
	Write       bool     `json:"write"`
	Aliases     []string `json:"aliases"`
	Source      string   `json:"source"`
}

// Manifest 是能力清单 —— 「我能做什么」的唯一真值，**必须现场生成**（SK-4）。
type Manifest struct {
	Tools   []Capability       `json:"tools"`
	Intents []IntentCapability `json:"intents"`
	Domains []DomainCapability `json:"domains"`
	Version string             `json:"version"`
}

// ExportManifest 从代码真实导出能力清单。
//
// P1 状态：**桩**（返回零值）——判据 SK-1..SK-6 应当全部为红。
// 形参带 registry 是为了让 SK-4 能"注入一个合成契约，清单必须跟着变"，
// 从而机械证明清单不是手写的。
func ExportManifest(reg *tools.Registry, sp *space.Registry) Manifest {
	return Manifest{}
}
