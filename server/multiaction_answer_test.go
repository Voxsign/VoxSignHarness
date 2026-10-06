//    G5-P2-7  back   :    clarification  to"first first   "giveout**  semantic**. 
//
// origstatus : resumeAsk  pipe   backorigsent(`orig  + "   : " +   `), 
// butorigsentbase then     -> again triggersend      ->     . 
// clarification    " first needfirst   ", but       hassemantic
// -- and G1/G3 sameclass  pt(      has  semantic),  is    example. 
//
// fixaftersemantic:    clarification   **thenisneed      **(cur new   refer ). 
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

	// useuserby  answer"first    "
	resp := postJSON(t, ts.URL+"/v1/tasks/"+id+"/answer", "", answerReq{Answer: "查一下库存"})
	resp.Body.Close()

	final := poll(t, ts, id)

	//  datais**  become    **:   backtosame sent   clarification. 
	//  allow  todiff ,  answer   (examplee.g. G2     "need    obj/file" --  is   , 
	// becauseas" store" has usedomain), also allow connect done. unique  connectaccept isorigly  . 
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
