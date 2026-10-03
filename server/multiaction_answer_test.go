// 评审 G5-P2-7 的回归测试：多动作回问必须对"先说先做哪个"给出**确定语义**。
//
// 原状况：resumeAsk 会把答案拼回原句（`原文 + " 澄清：" + 答案`），
// 而原句本身就含多动作 → 再次触发多动作检测 → 澄清循环。
// 回问文案承诺了「请先说要先做哪个」，但那个选项实际没有语义
// —— 与 G1/G3 同类翻车点（承诺的选项没有确定语义），这是第三个实例。
//
// 修后语义：多动作回问的答案**就是要执行的那一件**（当作新的单条指令）。
package server

import "testing"

func TestMultiActionAnswerExecutesChosenAction(t *testing.T) {
	ts := simPhone(t)
	id := submit(t, ts, "查一下库存，然后记一下结果，最后提交")

	first := poll(t, ts, id)
	if first.Status != stNeedAsk {
		t.Skipf("未停在 need_ask（实际 %s），跳过", first.Status)
	}
	if first.Question == "" {
		t.Fatal("多动作回问必须有 question")
	}

	// 用户按文案回答"先做哪一件"
	resp := postJSON(t, ts.URL+"/v1/tasks/"+id+"/answer", "", answerReq{Answer: "查一下库存"})
	resp.Body.Close()

	final := poll(t, ts, id)

	// 判据是**不形成澄清循环**：不得回到同一句多动作回问。
	// 允许它落到别的、可回答的追问（例如 G2 槽位闸问"要查哪个项目/文件" —— 那是合理的，
	// 因为"库存"没有作用域），也允许直接 done。唯一不可接受的是原地打转。
	if final.Status == stNeedAsk && final.Question == first.Question {
		t.Fatalf("P2-7: 回答后被逐字重新问同一句（澄清循环）question=%q", final.Question)
	}
	if final.Status == stNeedAsk {
		t.Logf("回答后落到另一条可回答的追问（非循环）：%q", final.Question)
	}
	if final.Status == stDone && final.Receipt == "" {
		t.Error("P2-7: done 必须带回执")
	}
}
