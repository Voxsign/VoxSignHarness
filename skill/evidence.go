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
	Paradigm  bool       // 范式优先（如"核电旧经验的默认正确性"）
	Detail    string
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
	}
	return c, nil
}

// IsVerified 报告该主张是否是**已执行 check 产生的**。
func (c Claim) IsVerified() bool { return c.kind == provExecuted && c.Executed }

// withTraceable / withParadigm 是便捷包装（仍不改变 kind 规则）。
func (c Claim) withTraceable(v bool) Claim { c.Traceable = v; return c }
func (c Claim) withParadigm(v bool) Claim  { c.Paradigm = v; return c }

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

func better(a, b Claim) bool {
	// ②③ 已执行/已查证优先（最强）
	if a.IsVerified() != b.IsVerified() {
		return a.IsVerified()
	}
	// ① 可追溯优先
	if a.Traceable != b.Traceable {
		return a.Traceable
	}
	// ④ 范式优先
	if a.Paradigm != b.Paradigm {
		return a.Paradigm
	}
	return false
}
