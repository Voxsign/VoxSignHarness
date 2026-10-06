// threshold_criteria_test.go
//
// ⚠️ **overwriteface(e.g. tgtnote)**: base dataonly   `asr/intent_model.go` **  file**. 
// **safety its file is alsohas  0.70 table diff semantic, base dataoverwrite to**(   ⑫b safety   ).  --  data⑫: same charface   table   semantic(   name). 
package asr

import (
	"os"
	"strings"
	"testing"
)

// ⑫a   semantic has name  (namechar same ⇒ will    call ,   face  )
func TestConfidenceThresholdsAreNamed(t *testing.T) {
	if ConfidenceThresholdAsk != 0.70 || ConfidenceThresholdModel != 0.70 {
		t.Errorf("[⑫] 阈值常量值不符: ask=%v model=%v", ConfidenceThresholdAsk, ConfidenceThresholdModel)
	}
	//  boundarysemantic(Lead  decide): **   at** ⇒    0.70  clarification
	if ConfidenceThresholdAsk <= 0 {
		t.Error("[⑫] 阈值应为正数")
	}
}

// ⑫b revexample:  code **  againhascharface  0.70**( then"onlymodify place"    same )
func TestNoRawThresholdLiteral(t *testing.T) {
	src, err := os.ReadFile("intent_model.go")
	if err != nil {
		t.Skipf("读取源码不可用：%v", err)
	}
	// only **  form  charface **(  define   0.70 is   value)
	for _, lit := range []string{">= 0.70", "< 0.70", "<= 0.70", "> 0.70"} {
		if strings.Contains(string(src), lit) {
			t.Errorf("[⑫] 比较式仍含字面量 %q —— 必须用具名常量", lit)
		}
	}
}
