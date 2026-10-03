// 手机 App ASR 模拟 · HTTP 契约套件（绿）。
//
// 与 e2e/asrsim_test.go 的区别：那一套直接调 pipeline.Run（同进程函数调用），
// 这一套**完整走手机真正会走的字节流**：
//
//	iPhone ASR 出文本 → POST /v1/tasks {text} → 202 {task_id}
//	                  → GET  /v1/tasks/{id} 轮询 → {task_id,status,role,question?,options?,receipt?,attribution?}
//
// 锁的是"手机屏幕上能看到什么"：
//   - 决策点必须有 question（否则是一张空白决策卡，用户无从回答）
//   - done 必须有 receipt，且四行齐全（M7"done 空渲染"的数据侧根因）
//   - 有候选按钮时每个按钮必须可点、可回传
//   - 连说几句不串号
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/pipeline"
)

// phoneView 是 INTERACT-v1 手机端轮询响应的真实形状（server.writeTaskView 产出）。
// 刻意不复用 taskState：那是服务端内部结构，含手机看不到的字段，
// 用它做断言会掩盖"手机实际收到什么"。
type phoneView struct {
	TaskID      string               `json:"task_id"`
	Status      string               `json:"status"`
	Role        string               `json:"role,omitempty"`
	Question    string               `json:"question,omitempty"`
	Options     []pipeline.AskOption `json:"options,omitempty"`
	Error       string               `json:"error,omitempty"`
	Receipt     string               `json:"receipt,omitempty"`
	Attribution contract.Attribution `json:"attribution,omitempty"`
	Reversible  bool                 `json:"reversible,omitempty"`
}

// simPhone 起一个测试用 harness，返回手机侧 HTTP 句柄。
func simPhone(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	t.Cleanup(ts.Close)
	return ts
}

// submit 模拟 App 提交一条 ASR 文本，返回 task_id。
func submit(t *testing.T, ts *httptest.Server, text string) string {
	t.Helper()
	resp := postJSON(t, ts.URL+"/v1/tasks", "", tasksPostReq{Text: text})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		t.Fatalf("提交 %q 应 202/200，实际 %d", text, resp.StatusCode)
	}
	var ack struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		t.Fatalf("解析提交响应失败: %v", err)
	}
	if ack.TaskID == "" {
		t.Fatalf("提交 %q 未返回 task_id（手机无法轮询）", text)
	}
	return ack.TaskID
}

// poll 轮询到决策点或终态（手机 App 的真实行为）。
func poll(t *testing.T, ts *httptest.Server, id string) phoneView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var view phoneView
	for time.Now().Before(deadline) {
		resp, err := http.Get(ts.URL + "/v1/tasks/" + id)
		if err != nil {
			t.Fatalf("轮询 %s 失败: %v", id, err)
		}
		err = json.NewDecoder(resp.Body).Decode(&view)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("解析任务视图失败: %v", err)
		}
		switch view.Status {
		case stDone, stCanceled, stNeedAsk, stNeedConfirm, stInterrupted:
			return view
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("任务 %s 超时未到决策点/终态: %+v", id, view)
	return view
}

// say 是"说一句"的完整往返。
func say(t *testing.T, ts *httptest.Server, text string) phoneView {
	t.Helper()
	return poll(t, ts, submit(t, ts, text))
}

// receiptLabels 是手机回执必须齐的四行（contract.RenderReceipt 的固定标签）。
var receiptLabels = []string{"动作：", "文件：", "结果：", "撤销："}

// asrFuzzyCorpus 是手机 ASR 常见的模糊文本（无音频，直接给文本）。
var asrFuzzyCorpus = []string{
	"推进项目",
	"继续",
	"接下来做什么",
	"那个功能还行",
	"刚才说的那个呢",
	"我要那个",
	"是不是可以了",
	"我想想",
	"先这样吧",
	"嗯那个呃记一下",
	"把这个改一下",
	"随便看看",
	"开始测试",
}

// TestASRSimPhoneFuzzyNeverBlankOrStuck 模糊输入不允许在手机上表现为"卡住"或"空白页"。
//
// 每个 ASR 模糊句都必须落到终态（done/canceled）或决策点（need_ask/need_confirm）；
// 决策点必须带问题文本，终态必须带回执 —— 否则手机端渲染出没有内容的一屏。
func TestASRSimPhoneFuzzyNeverBlankOrStuck(t *testing.T) {
	ts := simPhone(t)
	for _, text := range asrFuzzyCorpus {
		view := say(t, ts, text)
		switch view.Status {
		case stDone, stCanceled:
			if strings.TrimSpace(view.Receipt) == "" {
				t.Errorf("%q: 终态 %s 但 receipt 为空 —— 手机渲染空白回执卡", text, view.Status)
			}
		case stNeedAsk, stNeedConfirm:
			if strings.TrimSpace(view.Question) == "" {
				t.Errorf("%q: 决策点 %s 但 question 为空 —— 手机显示空白决策卡，用户无从回答",
					text, view.Status)
			}
		case stInterrupted:
			// 重启恢复态，手机有专门文案，不算空白
		default:
			t.Errorf("%q: 未到决策点或终态，状态 = %q（手机侧表现为卡住）", text, view.Status)
		}
	}
}

// TestASRSimPhoneReceiptAlwaysFourLines done 的回执四行必须齐全。
// 这是 M7"done 空渲染"的数据侧根因：任一行缺失，手机上就是一张残缺卡片。
func TestASRSimPhoneReceiptAlwaysFourLines(t *testing.T) {
	ts := simPhone(t)
	for _, text := range []string{"记一下明天开会", "查一下库存", "改一下", "开始测试", "随便看看"} {
		view := say(t, ts, text)
		if view.Status != stDone {
			t.Logf("%q: 停在 %s（决策点，另有用例覆盖）", text, view.Status)
			continue
		}
		if strings.TrimSpace(view.Receipt) == "" {
			t.Errorf("%q: done 但 receipt 为空", text)
			continue
		}
		for _, label := range receiptLabels {
			if !strings.Contains(view.Receipt, label) {
				t.Errorf("%q: 回执缺 %q:\n%s", text, label, view.Receipt)
			}
		}
	}
}

// TestASRSimPhoneAttributionAlwaysClassified 回执要能回答"这次是谁判断的"——归因六格必须有值。
func TestASRSimPhoneAttributionAlwaysClassified(t *testing.T) {
	ts := simPhone(t)
	for _, text := range []string{"记一下明天开会", "查一下库存"} {
		view := say(t, ts, text)
		if view.Status != stDone {
			continue
		}
		if strings.TrimSpace(view.Attribution.Class) == "" {
			t.Errorf("%q: done 但 attribution.class 为空，手机无法展示归因", text)
		}
	}
}

// TestASRSimPhoneAskOptionsWellFormed 有候选按钮时，每个按钮必须可点、可回传。
// 契约：ID 非空（answer 回传键）、Label 非空（用户要能读懂）、数量 ≤4（一屏一决策）。
func TestASRSimPhoneAskOptionsWellFormed(t *testing.T) {
	ts := simPhone(t)
	for _, text := range asrFuzzyCorpus {
		view := say(t, ts, text)
		for i, opt := range view.Options {
			if strings.TrimSpace(opt.ID) == "" {
				t.Errorf("%q: 候选[%d] ID 为空，点了无法回传答案: %+v", text, i, opt)
			}
			if strings.TrimSpace(opt.Label) == "" {
				t.Errorf("%q: 候选[%d] Label 为空，用户看到空按钮: %+v", text, i, opt)
			}
		}
		if len(view.Options) > 4 {
			t.Errorf("%q: 候选 %d 个，超过一屏一决策上限 4", text, len(view.Options))
		}
	}
}

// TestASRSimPhoneAnswerEndpointAccepts 澄清回答的管道本身要通：answer 返回 200、任务仍在追踪中。
// "回答后能否真正收敛到终态"是另一个问题，见 asrsim_gaps_test.go（当前为红）。
func TestASRSimPhoneAnswerEndpointAccepts(t *testing.T) {
	ts := simPhone(t)
	first := say(t, ts, "把这个改一下")
	if first.Status != stNeedAsk {
		t.Skipf("该句未停在 need_ask（实际 %s），跳过", first.Status)
	}
	resp := postJSON(t, ts.URL+"/v1/tasks/"+first.TaskID+"/answer", "",
		answerReq{Answer: "main.go"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("answer 应 200，实际 %d", resp.StatusCode)
	}
	after := poll(t, ts, first.TaskID)
	if after.TaskID != first.TaskID {
		t.Fatalf("续跑后 task_id 变了：%q → %q", first.TaskID, after.TaskID)
	}
	if after.Status == "" {
		t.Fatal("续跑后状态为空")
	}
}

// TestASRSimPhoneConcurrentTasksDoNotCross 连说几句不串号：每条任务拿到自己的回执。
// 对应 M7"回执行不稳定 id 导致 done 无反应"那一类问题的数据侧保证。
func TestASRSimPhoneConcurrentTasksDoNotCross(t *testing.T) {
	ts := simPhone(t)
	texts := []string{"记一下第一条", "记一下第二条", "查一下第三条"}
	ids := make([]string, 0, len(texts))
	for _, text := range texts {
		ids = append(ids, submit(t, ts, text))
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if seen[id] {
			t.Errorf("task_id 重复：%q 与前面的任务同号，回执会串", id)
		}
		seen[id] = true
		view := poll(t, ts, id)
		if view.Status != stDone {
			t.Errorf("%q(%s): 期望 done，实际 %s", texts[i], id, view.Status)
			continue
		}
		if strings.TrimSpace(view.Receipt) == "" {
			t.Errorf("%q(%s): 回执为空，用户看不到自己那条的结果", texts[i], id)
		}
		if view.TaskID != id {
			t.Errorf("%q: 返回的 task_id=%q 与请求的 %q 不一致", texts[i], view.TaskID, id)
		}
	}
}
