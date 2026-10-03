// evidence.go —— `basis` 判据族（证据优先级）+ `verified` 的**来源定义**。
//
// ⭐ 裁决（Lead 2026-10-03）：**`verified` 只能由「已执行的 check」产生；调用方口头给的一律 `hearsay`。**
// 实现手法沿用本项目最有效的一招：**类型上写不出来** —— `kind` 字段**未导出**，
// 外部包只能通过两个构造器产生 Claim，**无法伪造 verified**。
package skill

import "fmt"

// provenance 是证据来源等级（**未导出** ⇒ 外部只能经构造器产生）。
type provenance int

const (
	provClaimed  provenance = iota // 口头称（含调用方自称"已验证"）
	provExecuted                   // **已执行的 check** 产生
)

// Claim 是一条证据主张。
type Claim struct {
	kind      provenance // 未导出：外部写不出 verified
	Executed  bool       // 由构造器决定
	CheckID   string     // 已执行 check 的标识（Executed=true 时非空）
	Traceable bool       // 是否可追溯来源（URL/文件/命令）
	// paradigm **未导出**：按 Lead 裁决，`Paradigm` 与 `verified` 同口径 ——
	// **只能由「已执行的 check」产生**；口头主张"这是新范式"一律不算。
	paradigm bool
	Detail   string
}

// ClaimedByCaller 只能产生 hearsay（**无论调用方怎么自称**）。
func ClaimedByCaller(detail string) Claim {
	return Claim{kind: provClaimed, Executed: false, Detail: detail}
}

// ExecutedCheck 是**唯一**能产生 verified 的路径（需给出 check 标识）。
func ExecutedCheck(checkID string, passed bool, detail string) (Claim, error) {
	if checkID == "" {
		return Claim{}, fmt.Errorf("skill: ExecutedCheck 需要 checkID（verified 必须有来源）")
	}
	c := Claim{kind: provExecuted, Executed: true, CheckID: checkID, Detail: detail}
	if !passed {
		c.kind = provClaimed // 没通过 ⇒ 不算已查证
		c.Executed = false
	}
	return c, nil
}

// IsVerified 报告该主张是否是**已执行 check 产生的**。
func (c Claim) IsVerified() bool { return c.kind == provExecuted && c.Executed }

// WithTraceable 只影响"可追溯"标记（不涉及 verified/paradigm 的产生规则）。
func (c Claim) WithTraceable(v bool) Claim { c.Traceable = v; return c }

// RankByEvidence 实现 basis 判据族（**一次实现、四处生效**）：
//
//	① 可追溯来源 > 不可追溯
//	② 已执行 check > 口头称
//	③ 已查证 > 一方称
//	④ 范式优先（旧经验的默认正确性，仅在其余打平时生效）
//
// 返回最优主张；`degraded=true` 表示**没有任何已执行证据**（只能靠口头/范式）。
func RankByEvidence(claims []Claim) (Claim, bool) {
	if len(claims) == 0 {
		return Claim{}, true
	}
	best := claims[0]
	for _, c := range claims[1:] {
		if better(c, best) {
			best = c
		}
	}
	return best, !best.IsVerified()
}

// IsParadigm 报告该主张是否**经 check 认定**为范式。
func (c Claim) IsParadigm() bool { return c.paradigm }

// ExecutedParadigmCheck 与 ExecutedCheck 同口径：**范式认定也必须来自已执行的 check**。
func ExecutedParadigmCheck(checkID string, passed bool, detail string) (Claim, error) {
	c, err := ExecutedCheck(checkID, passed, detail)
	if err != nil {
		return Claim{}, err
	}
	if passed {
		c.paradigm = true
	}
	return c, nil
}

// ruleAppliers 把**族规则名**映射到**可执行的比较函数**（这就是"联动"）：
// 映射表给出规则名 ⇒ 这里必须真有对应实现，规则才可能改变排序。
var ruleAppliers = map[string]func(a, b Claim) int{
	RuleExecuted: func(a, b Claim) int { // ② 已执行 > 口头
		if a.IsVerified() == b.IsVerified() {
			return 0
		}
		if a.IsVerified() {
			return 1
		}
		return -1
	},
	RuleVerified: func(a, b Claim) int { // ③ 已查证 > 一方称（同 ②，语义别名）
		if a.IsVerified() == b.IsVerified() {
			return 0
		}
		if a.IsVerified() {
			return 1
		}
		return -1
	},
	RuleTraceable: func(a, b Claim) int { // ① 可追溯 > 不可追溯
		if a.Traceable == b.Traceable {
			return 0
		}
		if a.Traceable {
			return 1
		}
		return -1
	},
	RuleParadigm: func(a, b Claim) int { // ④ 范式优先（仅经 check 认定）
		if a.paradigm == b.paradigm {
			return 0
		}
		if a.paradigm {
			return 1
		}
		return -1
	},
}

// RulesFromMapping 只返回**映射成功**的规则名（manual 条**不参与**执行 —— 不许假装它在跑）。
func RulesFromMapping(ms []BasisMapping) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		for _, mp := range m.Mapped {
			if !seen[mp.Rule] {
				seen[mp.Rule] = true
				out = append(out, mp.Rule)
			}
		}
	}
	return out
}

// RankByEvidenceWith 用**给定规则集**排序（rules 为空 ⇒ 不应用任何规则，保持原序）。
// 这是"映射 ⇒ 执行"的联动点：映射出哪些规则，排序里就真的用哪些。
func RankByEvidenceWith(rules []string, claims []Claim) (Claim, bool) {
	if len(claims) == 0 {
		return Claim{}, true
	}
	best := claims[0]
	for _, c := range claims[1:] {
		for _, r := range rules {
			fn, ok := ruleAppliers[r]
			if !ok {
				continue // 未实现的规则名一律忽略（不许假装生效）
			}
			if fn(c, best) > 0 {
				best = c
				break
			}
			if fn(c, best) < 0 {
				break
			}
		}
	}
	return best, !best.IsVerified() // 默认全规则（与 RankByEvidenceWith 全规则等价）
}

func better(a, b Claim) bool {
	// ②③ 已执行/已查证优先（最强）
	if a.IsVerified() != b.IsVerified() {
		return a.IsVerified()
	}
	// ① 可追溯优先
	if a.Traceable != b.Traceable {
		return a.Traceable
	}
	// ④ 范式优先（**只有经 check 认定的范式**才参与；口头自称不算）
	if a.paradigm != b.paradigm {
		return a.paradigm
	}
	return false
}
