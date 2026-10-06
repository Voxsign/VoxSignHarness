//    P3  back   :      clarification   useuser" cancel",       has  semantic. 
//
// origstatus : resumeAsk only   edit/query/note/commit before ,  has"cancel"handle , 
// atisuseuser ing   "cancel" becurbecomeagain       --
//  coreferencewordtimebe  become" needdelete“cancel”"-> again    -> same  Ask -> by" recv "  ; 
//   coreferencewordtime become"   : cancel"-> heavynew  . close  all   , but  iserrorstate. 
package server

import "testing"

func TestIsCancelAnswer(t *testing.T) {
	for _, s := range []string{"取消", " 取消 ", "不用了", "算了", "不做了", "不要了", "cancel", "No", "n"} {
		if !isCancelAnswer(s) {
			t.Errorf("isCancelAnswer(%q) 应为 true", s)
		}
	}
	for _, s := range []string{"执行", "main.go", "确认", "是", "yes", ""} {
		if isCancelAnswer(s) {
			t.Errorf("isCancelAnswer(%q) 应为 false", s)
		}
	}
}

// TestCancelAtAskEndsTaskCleanly endtoend: to  clarificationanswer"cancel"->   cancel,  is" recv "error. 
func TestCancelAtAskEndsTaskCleanly(t *testing.T) {
	ts := simPhone(t)
	id := submit(t, ts, "不要删除那个文件")

	first := poll(t, ts, id)
	if first.Status != stNeedAsk {
		t.Skipf("该句未停在 need_ask（实际 %s），跳过", first.Status)
	}
	if first.Question == "" {
		t.Fatal("决策点必须有 question")
	}

	resp := postJSON(t, ts.URL+"/v1/tasks/"+id+"/answer", "", answerReq{Answer: "取消"})
	resp.Body.Close()

	final := poll(t, ts, id)
	if final.Status != stCanceled {
		t.Fatalf("回答「取消」后状态 = %s，期望 canceled", final.Status)
	}
	if final.Error != "" {
		t.Errorf("干净取消不该带 error，实际 %q（不应落到澄清未收敛路径）", final.Error)
	}
}
