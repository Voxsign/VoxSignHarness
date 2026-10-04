package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// SK-CHK-1 · 每个被 criteriaOnly 标为可机械化的 Check 名，必须有真实执行体。
//
// 为什么（2026-10-03）：判据原先只有 Check 名（声称可机械化），而判定器本身不存在
// ⇒ 「可机械化」是空话。本判据钉住这件事：标了就必须有东西真的去判。
func TestSKCHK1AllDeclaredChecksAreImplemented(t *testing.T) {
	cs := CriteriaFromKnowhow("arch-guardian", "v0.1.0", archGuardianKnowhow())
	seen := map[string]bool{}
	for _, c := range cs {
		if c.Manual || c.Check == "manual" || c.Check == "" {
			continue
		}
		seen[c.Check] = true
	}
	if len(seen) == 0 {
		t.Fatalf("[SK-CHK-1] 一条可机械化的判据都没有 ⇒ 判据本身失效")
	}
	for name := range seen {
		if !IsCheckImplemented(name) {
			t.Errorf("[SK-CHK-1] 判据声明 check=%q，但判定器未实现 ⇒ 「可机械化」是空话", name)
		}
	}
	t.Logf("[SK-CHK-1] 已声明 %d 个 check 名，全部有执行体 ✅", len(seen))
}

// SK-CHK-2 · 判不了 ⇒ 报错，不许当通过（三态纪律：算不出来 != 通过）。
func TestSKCHK2UnknownCheckErrorsNotPasses(t *testing.T) {
	if _, err := RunCheck("checkDoesNotExist", ".", nil); err == nil {
		t.Errorf("[SK-CHK-2] 未登记的判定器竟然没报错 ⇒ 会把「算不出来」混成「通过」")
	}
	if IsCheckImplemented("checkDoesNotExist") {
		t.Errorf("[SK-CHK-2] 未登记的判定器竟报「已实现」")
	}
	if _, err := RunCheck("checkWithinBoundary", ".", nil); err == nil {
		t.Errorf("[SK-CHK-2] 未给门禁结果却没报错 ⇒ 会把「没跑门禁」混成「未越界」")
	}
	if _, err := RunCheck("checkVerifiedOverHearsay", ".", nil); err == nil {
		t.Errorf("[SK-CHK-2] 未给证据类别却没报错")
	}
}

// SK-CHK-3 · checkADRExists 必须真的去看文件（不是默认 true/false）。
//
// 用临时目录构造两种参考系：有 ADR / 无 ADR ⇒ 结果必须不同。
func TestSKCHK3ADRCheckActuallyLooksAtFiles(t *testing.T) {
	empty := t.TempDir()
	r1, err := RunCheck("checkADRExists", empty, nil)
	if err != nil {
		t.Fatalf("[SK-CHK-3] 空目录判定失败: %v", err)
	}
	if r1.Passed {
		t.Errorf("[SK-CHK-3] 空目录竟然判「有 ADR」⇒ 判定器没真看文件")
	}

	withADR := t.TempDir()
	dir := filepath.Join(withADR, "docs", "adr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0001-use-go.md"), []byte("# ADR 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r2, err := RunCheck("checkADRExists", withADR, nil)
	if err != nil {
		t.Fatalf("[SK-CHK-3] 有 ADR 目录判定失败: %v", err)
	}
	if !r2.Passed {
		t.Errorf("[SK-CHK-3] 明明有 docs/adr/0001-use-go.md 却判「无 ADR」⇒ 口径没对上: %s", r2.Detail)
	}
	if r1.Passed == r2.Passed {
		t.Errorf("[SK-CHK-3] 两种参考系结果相同 ⇒ 判定器没真看文件（这是最要命的失效）")
	}
	t.Logf("[SK-CHK-3] 空目录=%v · 有ADR=%v ✅（判定器真看文件）", r1.Passed, r2.Passed)
}

// SK-CHK-4 · checkWithinBoundary 读门禁裁决，不自己重实现门禁。
func TestSKCHK4BoundaryCheckFollowsGateVerdict(t *testing.T) {
	ok, err := RunCheck("checkWithinBoundary", ".", map[string]string{"gate_allowed": "true"})
	if err != nil || !ok.Passed {
		t.Errorf("[SK-CHK-4] gate_allowed=true 应判通过: %+v err=%v", ok, err)
	}
	no, err := RunCheck("checkWithinBoundary", ".", map[string]string{
		"gate_allowed": "false", "gate_reason": "越界：目标不在已授权域"})
	if err != nil {
		t.Fatalf("[SK-CHK-4] 判定失败: %v", err)
	}
	if no.Passed {
		t.Errorf("[SK-CHK-4] gate_allowed=false 竟判通过 ⇒ 会放过越界")
	}
	if _, err := RunCheck("checkWithinBoundary", ".", map[string]string{"gate_allowed": "???"}); err == nil {
		t.Errorf("[SK-CHK-4] 无法解读的门禁结果没报错 ⇒ 会静默当通过")
	}
}
