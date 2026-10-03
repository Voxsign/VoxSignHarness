// threshold_criteria_test.go
//
// ⚠️ **覆盖面（如实标注）**：本判据只扫描 `asr/intent_model.go` **一个文件**。
// **全仓其它文件里是否还有裸 0.70 表达别的语义，本判据覆盖不到**（未做 ⑫b 全仓扫描）。 —— 判据⑫：同一字面量不得表达两个语义（必须具名）。
package asr

import (
	"os"
	"strings"
	"testing"
)

// ⑫a 两个语义各有具名常量（名字不同 ⇒ 将来可单独调整、影响面清晰）
func TestConfidenceThresholdsAreNamed(t *testing.T) {
	if ConfidenceThresholdAsk != 0.70 || ConfidenceThresholdModel != 0.70 {
		t.Errorf("[⑫] 阈值常量值不符: ask=%v model=%v", ConfidenceThresholdAsk, ConfidenceThresholdModel)
	}
	// 边界语义（Lead 裁决）：**严格小于** ⇒ 恰好 0.70 不回问
	if ConfidenceThresholdAsk <= 0 {
		t.Error("[⑫] 阈值应为正数")
	}
}

// ⑫b 反例：源码里**不得再有字面量 0.70**（否则"只改一处"会静默不同步）
func TestNoRawThresholdLiteral(t *testing.T) {
	src, err := os.ReadFile("intent_model.go")
	if err != nil {
		t.Skipf("读取源码不可用：%v", err)
	}
	// 只查**比较式里的字面量**（常量定义里的 0.70 是合法真值）
	for _, lit := range []string{">= 0.70", "< 0.70", "<= 0.70", "> 0.70"} {
		if strings.Contains(string(src), lit) {
			t.Errorf("[⑫] 比较式仍含字面量 %q —— 必须用具名常量", lit)
		}
	}
}
