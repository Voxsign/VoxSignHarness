package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checks.go —— **机械化判定器**（`Criterion.Check` 的执行体）。
//
// ⚠️ 为什么要有这个文件（2026-10-03）：
//   `criteriaOnly` 会给判据标 `Check` 名与 `Manual=false`（"可机械判定"），
//   但**判定器本身**原先不存在（`ExecutedCheck` 需要调用方传 `passed` 布尔）。
//   ⇒ 即：判据被**声称**可机械判定，而**没有人真的去判**。
//   ⇒ 本文件补上判定体 —— **"可机械化"的实质是"有东西真的去判它"**。
//
// 红线（沿用 `evidence.go`）：
//   · 判定器**必须真的去看东西**（文件 / 门禁结果），**不许默认 true**
//   · 判不了 ⇒ 返回 error，**不返回"通过"**（"算不出来" ≠ "通过"）

// CheckResult 是一次判定的结果。
type CheckResult struct {
	Passed bool
	Detail string // **人类可读的证据**（判了什么、在哪判的）
}

// CheckRunner 是判定器的统一签名。
//
// root 是仓库根（判定器要去看文件）；extra 供需要额外输入的判定器用（如门禁结果）。
type CheckRunner func(root string, extra map[string]string) (CheckResult, error)

// checkRegistry 登记全部机械化判定器。
//
// ⚠️ 新增判定器必须**同时**在此登记，并在 `TestAllChecksAreRegistered` 里被覆盖 ——
// 否则 `criteriaOnly` 标出来的 `Check` 名会**无人执行**（那正是本文件要修的病）。
var checkRegistry = map[string]CheckRunner{
	"checkADRExists":           checkADRExists,
	"checkWithinBoundary":      checkWithinBoundary,
	"checkVerifiedOverHearsay": checkVerifiedOverHearsay,
}

// RunCheck 执行一个具名判定器。
//
// ⚠️ 名字未登记 ⇒ **返回 error**（**不返回通过**）—— 未实现的判定器不许静默当绿。
func RunCheck(name, root string, extra map[string]string) (CheckResult, error) {
	r, ok := checkRegistry[name]
	if !ok {
		return CheckResult{}, fmt.Errorf("skill: 判定器 %q **未实现**（已登记 %d 个）⇒ 不许当通过", name, len(checkRegistry))
	}
	return r(root, extra)
}

// IsCheckImplemented 报告某个 check 名是否有**真实执行体**。
// 供 `vhs skill-ratio` 区分"标了可机械化"与"真的有判定器"。
func IsCheckImplemented(name string) bool {
	_, ok := checkRegistry[name]
	return ok
}

// -------------------- 判定器实现 --------------------

// checkADRExists 判「决策有 ADR」：**查文件**（target ④ 点名：「"决策有ADR"查文件」）。
//
// 判据：仓库里存在 ADR（架构决策记录）文件。
// 识别口径（**显式写出来，避免"看着像就算"**）：
//
//	· 路径含 `adr` 目录段（不区分大小写），扩展名 .md
//	· 或文件名形如 `ADR-<数字>` / `NNNN-<标题>.md` 且在 docs/ 下
//
// ⚠️ 2026-10-03 实测：本仓库**当前没有** ADR 文件 ⇒ 本判定器会判 `false`。
//
//	⇒ 那**不是判定器错**，是**产品缺 ADR 记录** —— 机械化把"人工觉得该有"变成"可查的事实"。
func checkADRExists(root string, _ map[string]string) (CheckResult, error) {
	if strings.TrimSpace(root) == "" {
		return CheckResult{}, fmt.Errorf("checkADRExists: 需要仓库根路径（**判不了，不是通过**）")
	}
	var found []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过读不了的目录（不因权限问题误判）
		}
		name := d.Name()
		if d.IsDir() {
			// 跳过明显的噪音目录
			switch name {
			case ".git", "node_modules", "vendor", ".calib-evidence":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		low := strings.ToLower(rel)
		// 口径 ①：路径里有 adr 目录段
		if strings.Contains(low, "/adr/") || strings.HasPrefix(low, "adr/") {
			found = append(found, rel)
			return nil
		}
		// 口径 ②：文件名形如 ADR-123 / 0001-xxx.md 且在 docs/ 下
		base := strings.ToUpper(name)
		if strings.Contains(low, "docs/") &&
			(strings.HasPrefix(base, "ADR-") || isNNNNDashed(name)) {
			found = append(found, rel)
		}
		return nil
	})
	if err != nil {
		return CheckResult{}, fmt.Errorf("checkADRExists: 遍历仓库失败: %w", err)
	}
	if len(found) == 0 {
		return CheckResult{
			Passed: false,
			Detail: "仓库里**没有** ADR 文件（口径：路径含 adr/ 目录段，或 docs/ 下 ADR-* / NNNN-*.md）",
		}, nil
	}
	return CheckResult{
		Passed: true,
		Detail: fmt.Sprintf("找到 %d 个 ADR 文件，例如 %s", len(found), strings.Join(head(found, 3), " · ")),
	}, nil
}

// checkWithinBoundary 判「约束未越界」：**用域门禁**（target ④ 点名）。
//
// 域门禁的裁决由 `pipeline` 产生（`verdict.Allowed` / `BOUNDARY_VIOLATION`）。
// 本判定器**不重新实现门禁**，只读调用方传进来的门禁结果（`extra["gate_allowed"]`）。
//
// ⚠️ 未传门禁结果 ⇒ **返回 error**（"没跑门禁" ≠ "未越界"）。
func checkWithinBoundary(_ string, extra map[string]string) (CheckResult, error) {
	v, ok := extra["gate_allowed"]
	if !ok {
		return CheckResult{}, fmt.Errorf(
			"checkWithinBoundary: 未提供域门禁结果（extra[gate_allowed]）⇒ **判不了，不是通过**；" +
				"域门禁裁决在 pipeline（verdict.Allowed / BOUNDARY_VIOLATION）")
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "allowed":
		reason := extra["gate_reason"]
		return CheckResult{Passed: true, Detail: "域门禁裁决 = 放行（verdict.Allowed）" + suffix(reason)}, nil
	case "false", "0", "no", "denied":
		reason := extra["gate_reason"]
		return CheckResult{Passed: false, Detail: "域门禁裁决 = **BOUNDARY_VIOLATION**" + suffix(reason)}, nil
	default:
		return CheckResult{}, fmt.Errorf("checkWithinBoundary: 门禁结果 %q 无法解读 ⇒ **判不了，不是通过**", v)
	}
}

// checkVerifiedOverHearsay 判「已查证优先于一方称」（既有规则，补上执行体）。
//
// 已执行 check 产生的证据 > 口头称。`extra["evidence_kind"]` 取值：verified | hearsay。
func checkVerifiedOverHearsay(_ string, extra map[string]string) (CheckResult, error) {
	k, ok := extra["evidence_kind"]
	if !ok {
		return CheckResult{}, fmt.Errorf("checkVerifiedOverHearsay: 未提供证据类别（extra[evidence_kind]）⇒ **判不了，不是通过**")
	}
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "verified":
		return CheckResult{Passed: true, Detail: "证据来自**已执行 check**（ExecutedCheck）"}, nil
	case "hearsay":
		return CheckResult{Passed: false, Detail: "证据仅是**一方称**（hearsay）⇒ 不足以支撑该判据"}, nil
	default:
		return CheckResult{}, fmt.Errorf("checkVerifiedOverHearsay: 证据类别 %q 无法解读 ⇒ **判不了，不是通过**", k)
	}
}

// -------------------- 小工具 --------------------

func isNNNNDashed(name string) bool {
	// 形如 0001-some-title.md
	i := strings.IndexByte(name, '-')
	if i != 4 {
		return false
	}
	for _, c := range name[:4] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func head(xs []string, n int) []string {
	if len(xs) <= n {
		return xs
	}
	return xs[:n]
}

func suffix(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "（" + s + "）"
}
