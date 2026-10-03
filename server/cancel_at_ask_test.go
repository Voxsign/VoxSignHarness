// 评审 P3 的回归测试：否定仲裁的回问文案让用户「说取消」，这个选项必须有确定语义。
//
// 原状况：resumeAsk 只映射 edit/query/note/commit 前缀，没有「取消」处理器，
// 于是用户照着文案说「取消」会被当成又一轮澄清答案 ——
// 含指代词时被替换成「不要删除“取消”」→ 再判否定 → 同一 Ask → 以"未收敛"报错；
// 不含指代词时拼成「 澄清：取消」→ 重新提问。结果虽都不执行，但走的是错误态。
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

// TestCancelAtAskEndsTaskCleanly 端到端：对否定回问回答「取消」→ 干净取消，不是"未收敛"错误。
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
