package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sampleDoc is a small requirements doc used to exercise the deterministic
// extractors. It contains headings so the plan renderer has something to pull.
const sampleDoc = "# 需求概览\n\n产品需要一个可运行的语音服务。\n\n## 端点\n\n必须实现 /v1/health 与 /v1/process。\n\n## 数据落盘\n\n所有记录 append-only。\n"

func TestDetectLang(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Lang
	}{
		{"empty defaults to en", "", LangEN},
		{"plain english", "build a service with health endpoint", LangEN},
		{"chinese sentence", "帮我实现一个语音服务，包含健康检查端点", LangZH},
		{"chinese punctuation only", "（测试）、。「引号」", LangZH},
		{"mixed leads with cjk counts zh", "实现 backend API", LangZH},
		{"ascii slug", "harness-output/impl", LangEN},
	}
	for _, c := range cases {
		if got := DetectLang(c.text); got != c.want {
			t.Errorf("%s: DetectLang(%q) = %q, want %q", c.name, c.text, got, c.want)
		}
	}
}

func TestEffectiveLangPriority(t *testing.T) {
	// Explicit override wins over detection.
	for _, override := range []Lang{LangEN, LangZH} {
		o := &Options{Lang: string(override)}
		// Even CJK input cannot flip an explicit override.
		if got := o.EffectiveLang("随便一段中文"); got != override {
			t.Errorf("override %q lost to detection: got %q", override, got)
		}
	}
	// Empty Lang -> detect from input.
	o := &Options{}
	if got := o.EffectiveLang("build the thing"); got != LangEN {
		t.Errorf("empty Lang + english input = %q, want en", got)
	}
	if got := o.EffectiveLang("做一个东西"); got != LangZH {
		t.Errorf("empty Lang + cjk input = %q, want zh", got)
	}
	// Unknown Lang value falls back to detection (never empty/broken).
	oBad := &Options{Lang: "fr"}
	if got := oBad.EffectiveLang("bonjour"); got != LangEN {
		t.Errorf("unknown Lang + latin input = %q, want en (detect fallback)", got)
	}
	// Nil Options must not panic and must detect.
	var nilOpts *Options
	if got := nilOpts.EffectiveLang("你好"); got != LangZH {
		t.Errorf("nil Options + cjk = %q, want zh", got)
	}
}

func TestDeterministicSummaryBilingual(t *testing.T) {
	names := []string{"a.md", "b.md"}
	en := deterministicSummary(LangEN, "Demo", names, "merged body")
	if !strings.Contains(en, "## Overview") {
		t.Errorf("en summary missing Overview header:\n%s", en)
	}
	if !strings.Contains(en, "source documents:") {
		t.Errorf("en summary missing included-documents line:\n%s", en)
	}
	if !strings.Contains(en, "a.md") || !strings.Contains(en, "b.md") {
		t.Errorf("en summary missing source names:\n%s", en)
	}
	if strings.Contains(en, "概览") {
		t.Errorf("en summary leaked Chinese content:\n%s", en)
	}

	zh := deterministicSummary(LangZH, "Demo", names, "merged body")
	if !strings.Contains(zh, "## 概览") {
		t.Errorf("zh summary missing 概览 header:\n%s", zh)
	}
	if !strings.Contains(zh, "份源文档") {
		t.Errorf("zh summary missing included-documents line:\n%s", zh)
	}
	if strings.Contains(zh, "Overview") {
		t.Errorf("zh summary leaked English header:\n%s", zh)
	}
}

func TestDeterministicPlanBilingual(t *testing.T) {
	en := deterministicImplementPlan(LangEN, "MyService", sampleDoc)
	for _, want := range []string{
		"## Implementation goal",
		"- Product: MyService",
		"## Requirements highlights (deterministic extraction)",
		"| Module | Responsibility | Key interface |",
		"## Acceptance mapping",
		"## Implementation steps (skeleton)",
	} {
		if !strings.Contains(en, want) {
			t.Errorf("en plan missing %q:\n%s", want, en)
		}
	}
	// Headings from the doc must be extracted regardless of language.
	if !strings.Contains(en, "需求概览") {
		t.Errorf("en plan did not extract doc headings:\n%s", en)
	}
	if strings.Contains(en, "## 实现目标") {
		t.Errorf("en plan leaked Chinese section:\n%s", en)
	}

	zh := deterministicImplementPlan(LangZH, "MyService", sampleDoc)
	for _, want := range []string{
		"## 实现目标",
		"- 产品：MyService",
		"## 需求要点（确定性提取）",
		"| 模块 | 职责 | 关键接口 |",
		"## 验收映射",
		"## 实施步骤（骨架）",
	} {
		if !strings.Contains(zh, want) {
			t.Errorf("zh plan missing %q:\n%s", want, zh)
		}
	}
	if strings.Contains(zh, "Implementation goal") {
		t.Errorf("zh plan leaked English section:\n%s", zh)
	}

	// No-headings branch is localized too.
	if noHeadings := deterministicImplementPlan(LangZH, "X", "plain text only"); !strings.Contains(noHeadings, "建议人工复核") {
		t.Errorf("zh no-headings branch not localized:\n%s", noHeadings)
	}
}

func TestDeterministicSkeletonBilingual(t *testing.T) {
	en := deterministicImplementSkeleton(LangEN, "Demo Product", sampleDoc)
	for _, f := range []string{"main.go", "router.go", "domain.go", "README.md", "go.mod"} {
		if _, ok := en[f]; !ok {
			t.Fatalf("en skeleton missing %s", f)
		}
	}
	if !strings.Contains(en["main.go"], "Target product: Demo Product") {
		t.Errorf("en main.go missing target product:\n%s", en["main.go"])
	}
	if !strings.Contains(en["main.go"], "listen address (loopback by default; non-loopback refused)") {
		t.Errorf("en main.go missing localized flag usage:\n%s", en["main.go"])
	}
	if !strings.Contains(en["router.go"], "skeleton not implemented") {
		t.Errorf("en router.go missing not-implemented literal:\n%s", en["router.go"])
	}
	if !strings.Contains(en["README.md"], "(Harness-generated code skeleton)") {
		t.Errorf("en README missing title suffix:\n%s", en["README.md"])
	}
	if strings.Contains(en["domain.go"], "TODO(实现阶段)") {
		t.Errorf("en domain.go leaked Chinese TODO:\n%s", en["domain.go"])
	}

	zh := deterministicImplementSkeleton(LangZH, "演示产品", sampleDoc)
	if !strings.Contains(zh["main.go"], "目标产品：演示产品") {
		t.Errorf("zh main.go missing target product:\n%s", zh["main.go"])
	}
	if !strings.Contains(zh["main.go"], "监听地址") {
		t.Errorf("zh main.go missing localized flag usage:\n%s", zh["main.go"])
	}
	if !strings.Contains(zh["router.go"], "骨架未实现") {
		t.Errorf("zh router.go missing not-implemented literal:\n%s", zh["router.go"])
	}
	if !strings.Contains(zh["README.md"], "（Harness 产出代码骨架）") {
		t.Errorf("zh README missing title suffix:\n%s", zh["README.md"])
	}
	if !strings.Contains(zh["domain.go"], "TODO(实现阶段)") {
		t.Errorf("zh domain.go missing Chinese TODO:\n%s", zh["domain.go"])
	}
}

// TestSkeletonGeneratedGoCompiles is the strongest i18n assertion: the generated
// main.go/router.go/domain.go must build with `go build` in BOTH languages, so a
// bad interpolation (e.g. an unescaped quote in a localized string) cannot slip
// through as a "rendered string" test.
func TestSkeletonGeneratedGoCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go-build of generated skeleton in -short mode")
	}
	for _, lang := range []Lang{LangEN, LangZH} {
		skel := deterministicImplementSkeleton(lang, "Compile Check", sampleDoc)
		dir := t.TempDir()
		for name, body := range skel {
			if name == "README.md" {
				continue // not Go; skip for compile gate
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatalf("%s: write %s: %v", lang, name, err)
			}
		}
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s skeleton failed `go build`:\n%s", lang, out)
		}
	}
}
