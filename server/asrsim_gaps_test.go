//go:build asrsimgap

// mobile App ASR    · HTTP  already   (  -- M8   ). 
//
// use  tgt   , keep  `./build.sh`(in   go test ./...) accept  : 
//
//	go test -tags asrsimgap ./server -run TestASRSimGap -v
package server

import (
	"testing"
)

// TestASRSimGapPhoneAskResumeLoops    G8:   continue  recv  --    . 
//
//  now(HTTP  , mobile  path): 
//
//	POST /v1/tasks        {"text":"pipe  modify under"}
//	-> need_ask            "   "  "refer is    again    pt"
//	POST .../answer       {"answer":"main.go"}
//	-> need_ask             char same   
//	…  ,   out  
//
// rootbecause: handleAnswer pipecontinue  base become `orig  + "   : " +   `, but refer  by
// CorrectedText   "  "    ; Recent asempty(   haschangebecome    ), 
// atis refer again writeoutsame sent Ask.   haspipe   give refer, also has"same   
// heavy  N  then  /  "   . 
//
//   : mobile to   thenagainalso  out  -- langaudio   needrequire  placedisconnect . 
//   modify : pipe    note as refer    (or connectpipe Target  become  ), 
// andgiveclarification     (  2   no   ->    or    andgive     ). 
func TestASRSimGapPhoneAskResumeLoops(t *testing.T) {
	ts := simPhone(t)
	id := submit(t, ts, "把这个改一下")

	first := poll(t, ts, id)
	if first.Status != stNeedAsk {
		t.Skipf("该句未停在 need_ask（实际 %s），当前无法复现本缺口", first.Status)
	}

	resp := postJSON(t, ts.URL+"/v1/tasks/"+id+"/answer", "", answerReq{Answer: "main.go"})
	resp.Body.Close()

	second := poll(t, ts, id)
	if second.Status == stNeedAsk && second.Question == first.Question {
		t.Errorf("GAP G8: 澄清续跑不收敛 —— 回答 %q 后仍被逐字追问同一句：%q\n"+
			"手机走到这一屏就走不出去了（need_ask → answer → need_ask 死循环）",
			"main.go", second.Question)
	}
	if second.Status == stDone && second.Receipt == "" {
		t.Errorf("GAP G8: 澄清续跑后 done 却无回执，手机看不到结果")
	}
}
