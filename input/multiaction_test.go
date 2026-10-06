//        boundary  (   G5,  after  ). 
//
// G5      : "  under store, howeverafter  underclose ,  after  "only      in intent, 
// its   **    also  show**, useuserbyas   all . 
//
//  data  use**  linkconnectword**(howeverafter/connecting/ after…)but is"sent  outnow    word"--
// M7    sentsametime  TEST and QUERY wordbutis seg lang  ,   keepkeep  Ask
// ( hasback  pipeline.TestColloquialQuestionNoReferAsk). 
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestMultiActionAsksForOrder G5  line:  sent     -> clarificationfirst   ,      . 
func TestMultiActionAsksForOrder(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"查一下库存，然后记一下结果，最后提交",
		"跑一下测试然后提交这批改动",
		"把主栈改成中文然后部署到服务器",
		"把报价模板改成新的公司抬头，然后跑一下测试",
		"记一下明天开会然后查一下上次的报价",
	} {
		got := c.ClassifyTask(text)
		if got.Ask == "" {
			t.Errorf("G5: %q 未回问（intent=%s）—— 其余动作会被静默丢弃", text, got.Intent)
		}
		if got.Conflict != contract.ConflictMultiAction {
			t.Errorf("%q: conflict = %q，期望 %q", text, got.Conflict, contract.ConflictMultiAction)
		}
	}
}

// TestMultiActionLongStatementExempt M7    lang sent  be    . 
func TestMultiActionLongStatementExempt(t *testing.T) {
	m7 := "我现在测试一下，看看效果怎么样，如果这个效果好，我们就继续推进，就是重点是把这个能力建立起来"
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask(m7)
	if got.Conflict == contract.ConflictMultiAction {
		t.Errorf("M7 口语长句被误判为多动作（conflict=%q）", got.Conflict)
	}
	if got.Ask != "" {
		t.Errorf("M7 长句不得回问（既有回归 TestColloquialQuestionNoReferAsk）：Ask=%q", got.Ask)
	}
	if n, labels, had := multiActionClauses(m7); multiActionTrips(n, had) {
		t.Errorf("M7 长句被判多动作 %d 段 %v —— 判据应基于顺序连接词", n, labels)
	}
}

// TestSingleActionUnchanged    sent  accept  . 
func TestSingleActionUnchanged(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantIntent string }{
		{"跑一下测试", contract.IntentTest},
		{"查一下库存", contract.IntentQuery},
		{"把报价模块改成中文", contract.IntentEdit},
		{"提交这批改动", contract.IntentCommit},
		{"记一下这个想法", contract.IntentNote},
	}
	for _, tc := range cases {
		got := c.ClassifyTask(tc.text)
		if got.Intent != tc.wantIntent {
			t.Errorf("%q: 意图 = %q，期望 %q（多动作检测误伤）", tc.text, got.Intent, tc.wantIntent)
		}
		if got.Conflict == contract.ConflictMultiAction {
			t.Errorf("%q 被误判为多动作", tc.text)
		}
	}
}

// TestMultiActionIntentsUnit  num   : bylinkconnectword splitafternum same  . 
func TestMultiActionIntentsUnit(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"查一下库存，然后记一下结果，最后提交", 3},
		{"跑一下测试然后提交这批改动", 2},
		{"跑一下测试", 1},
		{"查一下库存", 1},
		{"", 0},
	}
	for _, tc := range cases {
		if got, labels, _ := multiActionClauses(tc.text); got != tc.want {
			t.Errorf("multiActionClauses(%q) 分句数 = %d，期望 %d（%v）",
				tc.text, got, tc.want, labels)
		}
	}
}

// TestMultiActionDoesNotShadowOrchestrate orchestratetaskbase is   , alreadyby    ,   again . 
func TestMultiActionDoesNotShadowOrchestrate(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	text := "把全部沟通记录和设计文档整理成《全景开发文档》并保存提交"
	got := c.ClassifyTask(text)
	if got.Conflict == contract.ConflictMultiAction {
		t.Errorf("编排任务被多动作检测抢走：intent=%s conflict=%q", got.Intent, got.Conflict)
	}
}

// ---------------------------------------------------------------------------
//    G5-P0-2 / P1-3 / P1-4  revexampleback . 
// ---------------------------------------------------------------------------

// TestMultiActionSameIntentStillAsks    P0-2: **same  intentoutnow  **alsois   . 
// "first  A again  B"ifby" sameintentnum" heavy  become 1 ->   Ask ->          . 
func TestMultiActionSameIntentStillAsks(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"先查 A 再查 B",
		"先跑单元测试再跑集成测试",
		"先提交 A 再提交 B",
	} {
		got := c.ClassifyTask(text)
		if got.Conflict != contract.ConflictMultiAction {
			t.Errorf("P0-2: %q 未判多动作（intent=%s conflict=%q）—— 第二个动作会被静默丢弃",
				text, got.Intent, got.Conflict)
		}
		if got.Ask == "" {
			t.Errorf("P0-2: %q 必须回问，实际 Ask 为空", text)
		}
	}
}

// TestOrchestrateNotInterceptedByMultiAction    P1-3: orchestratetask**  **       . 
// orig now "   haslinkconnectword"only be   --   "howeverafteragain"then  edorchestrate. 
func TestOrchestrateNotInterceptedByMultiAction(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	for _, text := range []string{
		"把全部沟通记录和设计文档整理成《全景开发文档》并保存提交",
		"把全部沟通记录和设计文档整理成《全景开发文档》，然后再保存提交",
	} {
		got := c.ClassifyTask(text)
		if got.Conflict == contract.ConflictMultiAction {
			t.Errorf("P1-3: 编排任务被多动作检测抢走：%q（intent=%s）", text, got.Intent)
		}
	}
}

// TestClauseActionsNoMasking    P1-4:   splitsent in **safety **  allneed in. 
// orig nowbywordtable  onlyget   , but queryTriggers   char" / / "and   Debug/Edit ofbefore. 
func TestClauseActionsNoMasking(t *testing.T) {
	acts := clauseActions("查一下这个报错")
	has := func(want string) bool {
		for _, a := range acts {
			if a == want {
				return true
			}
		}
		return false
	}
	if !has(contract.IntentQuery) || !has(contract.IntentDebug) {
		t.Errorf("P1-4: 「查一下这个报错」应同时含 QUERY 与 DEBUG，实际 %v（DEBUG 被 QUERY 吞掉）", acts)
	}
	//  num  be masking   
	if n, _, _ := multiActionClauses("改一下顺便查一下，然后再看看"); n < 2 {
		t.Errorf("P1-4: 三个分句（改/查/看）应计为 ≥2，实际 %d", n)
	}
}

// ---------------------------------------------------------------------------
//    G5-P2-5 / P2-6  revexampleback . 
// ---------------------------------------------------------------------------

// TestMultiActionNoConnectorReviewP2_5    P2-5: nolinkconnectword  langandlist. 
// ASR  pipelinkconnectword  , only  idsplit timeneedrequire >=3 seg(bykeep  M7  lang sent). 
func TestMultiActionNoConnectorReviewP2_5(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("查一下库存，记一下结果，提交")
	if got.Conflict != contract.ConflictMultiAction {
		t.Errorf("P2-5: 无连接词的三段并列未判多动作（intent=%s conflict=%q）—— 会只执行第一件",
			got.Intent, got.Conflict)
	}
	if got.Ask == "" {
		t.Error("P2-5: 必须回问")
	}
}

// TestQuotedSpanNotSplitReviewP2_6    P2-6:  id  "howeverafter"isbe use  base,  islinkconnectword. 
func TestQuotedSpanNotSplitReviewP2_6(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	got := c.ClassifyTask("把提示语改成「然后提交」")
	if got.Conflict == contract.ConflictMultiAction {
		t.Errorf("P2-6: 引号内容被当连接词切分，误报多动作（intent=%s）", got.Intent)
	}
	if n, _, _ := multiActionClauses("把提示语改成「然后提交」"); n >= 2 {
		t.Errorf("P2-6: stripQuotedSpans 未生效，切出 %d 段", n)
	}
}

// TestSelfCorrectionIsNotMultiAction   fixposchain is   ( berev fixpos,  is   ). 
func TestSelfCorrectionIsNotMultiAction(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	text := "把标题改成中文，不对，改成英文，说错了，改成阿拉伯语"
	got := c.ClassifyTask(text)
	if got.Conflict == contract.ConflictMultiAction {
		t.Errorf("修正链被误判为多动作（intent=%s）", got.Intent)
	}
	if got.Intent != contract.IntentEdit || got.Params["value"] != "阿拉伯语" {
		t.Errorf("修正链应走 G7 逻辑得 EDIT/阿拉伯语，实际 intent=%s value=%q",
			got.Intent, got.Params["value"])
	}
}

// TestMultiActionTripsUnit   anddecide split after  value  . 
func TestMultiActionTripsUnit(t *testing.T) {
	cases := []struct {
		n       int
		hadConn bool
		want    bool
	}{
		{2, true, true},   // haslinkconnectword, 2 segi.e. 
		{2, false, false}, //  tgtpt, 2 seg  (keep  M7  sent)
		{3, false, true},  //  tgtpt, 3 seg    
		{1, false, false},
	}
	for _, tc := range cases {
		if got := multiActionTrips(tc.n, tc.hadConn); got != tc.want {
			t.Errorf("multiActionTrips(%d, %v) = %v，期望 %v", tc.n, tc.hadConn, got, tc.want)
		}
	}
}
