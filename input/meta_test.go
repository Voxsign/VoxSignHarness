//  refer     boundary  (   G3). 
//
// G3      : "openstart  "be  TEST 0.85 and test_kind="go test ./..." -- **      **. 
// but sent  to    is" in  stage", isto control,  is    . 
//
//  boundary close is** sent  **: M7     "  openstart    under, connectunder pipe obj  raise "
//  be    ,  objalreadypipe  becomeback useexample(pipeline.TestCodexNineRegressions   8  , 
// disconnectlang Ask asempty).  by refer onlyto** sent**occur  --  sent  "openstart/  "is  . 
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestMetaInstructionAsksForDisambiguation G3  line:  sent refer need  , and    TEST. 
func TestMetaInstructionAsksForDisambiguation(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{"开始测试", "继续测试", "开始跑测试", "继续部署"} {
		got := c.ClassifyTask(text)
		if got.Intent == contract.IntentTest {
			t.Errorf("G3: %q 被判 TEST —— 元指令被当作可执行命令（test_kind=%q）",
				text, got.Params["test_kind"])
		}
		if got.Intent != contract.IntentAsk {
			t.Errorf("%q: 意图 = %q，期望 ASK（歧义不猜）", text, got.Intent)
		}
		if got.Conflict != contract.ConflictMeta {
			t.Errorf("%q: conflict = %q，期望 %q", text, got.Conflict, contract.ConflictMeta)
		}
		if got.Ask == "" {
			t.Errorf("%q: 元指令消歧必须带非空 ask", text)
		}
	}
}

// TestMetaLongSentenceExempt  sent  :  sent  "openstart/  "is  ,  iscontrolrefer . 
func TestMetaLongSentenceExempt(t *testing.T) {
	long := "我想开始认真测一下，接下来把项目推进起来"
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask(long)
	if got.Conflict == contract.ConflictMeta {
		t.Errorf("长句被误判为元指令：conflict=%q intent=%s", got.Conflict, got.Intent)
	}
	if got.Ask != "" {
		t.Errorf("长句元指令不得回问（M7 真机回归，TestCodexNineRegressions#8）：Ask=%q", got.Ask)
	}
}

// TestMetaRequiresActionWord   refer no      ,      sent. 
func TestMetaRequiresActionWord(t *testing.T) {
	for _, text := range []string{"开始", "继续", "暂停"} {
		if _, ok := metaInstruction(text); ok {
			t.Errorf("%q 无动作词，不应判元指令", text)
		}
	}
	// has  wordonly 
	for _, text := range []string{"开始测试", "继续部署"} {
		if _, ok := metaInstruction(text); !ok {
			t.Errorf("%q 有动作词，应判元指令", text)
		}
	}
}

// TestMetaDoesNotBreakNormalCommands pos refer  accept  . 
func TestMetaDoesNotBreakNormalCommands(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"跑一下测试", contract.IntentTest},
		{"部署到服务器", contract.IntentDeploy},
		{"记一下这个想法", contract.IntentNote},
		{"提交这批改动", contract.IntentCommit},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.wantIntent {
			t.Errorf("%q: 意图 = %q，期望 %q（元指令仲裁误伤）", tc.text, got.Intent, tc.wantIntent)
		}
		if got.Conflict == contract.ConflictMeta {
			t.Errorf("%q 被误判为元指令", tc.text)
		}
	}
}

// TestNegationBeatsMeta    firstat refer : " needopenstart  " first   . 
func TestNegationBeatsMeta(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("不要开始测试")
	if got.Conflict != contract.ConflictNegation {
		t.Errorf("conflict = %q，期望 %q（否定应优先于元指令）", got.Conflict, contract.ConflictNegation)
	}
}

// ---------------------------------------------------------------------------
//    G3-P1  revexampleback : charnum   data    toall . 
// ---------------------------------------------------------------------------

// TestMetaMissedCasesReviewP1 origfirst **  **  refer ( then       ). 
func TestMetaMissedCasesReviewP1(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"好的，开始测试",
		"我们先开始测试吧好不好",
		"开始测试一下好不好",
		"嗯，开始测试",
	} {
		got := c.ClassifyTask(text)
		if got.Intent == contract.IntentTest {
			t.Errorf("P1 漏判: %q 仍被判 TEST（会真的跑 go test ./...）", text)
		}
		if got.Conflict != contract.ConflictMeta {
			t.Errorf("P1 漏判: %q conflict=%q，期望 %q", text, got.Conflict, contract.ConflictMeta)
		}
	}
}

// TestMetaFalsePositiveCasesReviewP1 origfirst **  ** pos refer ( thenpipe    changebecomeclarification). 
func TestMetaFalsePositiveCasesReviewP1(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"开始部署到沙特",
		"开始部署到服务器",
		"继续改报价",
		"接着删除那个文件",
	} {
		got := c.ClassifyTask(text)
		if got.Conflict == contract.ConflictMeta {
			t.Errorf("P1 误判: %q 被当元指令（真实指令被拦成回问）intent=%s", text, got.Intent)
		}
	}
}

// TestMetaActionTextReviewP4 " stop"classbefore   be become"continuecontinue  ". 
func TestMetaActionTextReviewP4(t *testing.T) {
	if text := metaActionText("暂停"); text == "继续推进" {
		t.Error("P4: 「暂停」不该套用「继续推进」文案")
	}
	if text := metaActionText("开始"); text != "继续推进" {
		t.Errorf("P4: 「开始」文案 = %q，期望「继续推进」", text)
	}
}
