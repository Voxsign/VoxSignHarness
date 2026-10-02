//go:build asrsimgap

// 手机 App ASR 模拟 · HTTP 层已知缺口（红 —— M8 驱动）。
//
// 用构建标签隔离，保证 `./build.sh`（内部跑 go test ./...）不受影响：
//
//	go test -tags asrsimgap ./server -run TestASRSimGap -v
package server

import (
	"testing"
)

// TestASRSimGapPhoneAskResumeLoops 缺口 G8：澄清续跑不收敛 —— 死循环。
//
// 复现（HTTP 层，手机真实路径）：
//
//	POST /v1/tasks        {"text":"把这个改一下"}
//	→ need_ask            "你说的「这个」指的是哪个？请再说清楚一点"
//	POST .../answer       {"answer":"main.go"}
//	→ need_ask            逐字相同的问题
//	…往复，永远出不来
//
// 根因：handleAnswer 把续跑文本拼成 `原文 + " 澄清：" + 答案`，而 refer 仍以
// CorrectedText 里的"这个"去找候选；Recent 为空（答案没有变成候选来源），
// 于是 refer 再次写出同一句 Ask。既没有把答案喂给 refer，也没有"同一问题
// 重复 N 次就升级/放弃"的退避。
//
// 影响：手机走到这一屏就再也走不出去 —— 语音闭环在需求澄清处断掉。
// 最小改法：把澄清答案注册为 refer 的候选（或直接把 Target 钉成答案），
// 并给回问加轮次退避（第 2 次仍无进展 → 换策略或明确放弃并给可执行建议）。
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
