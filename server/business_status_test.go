package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/config"
)

// initGitRepo turns dir into a real git repository with one uncommitted (untracked)
// file, so a COMMIT intent's `git add -A && git commit` genuinely succeeds. Without
// this, the confirm-lifecycle test ran git in a non-repository temp dir, the commit
// always failed, and the old false-success code reported done anyway — masking it.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("vhs test\n"), 0o644); err != nil {
		t.Fatalf("写夹具文件失败: %v", err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git 夹具 %v 失败: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "vhs@example.com")
	run("config", "user.name", "VHS Test")
}

// postTaskBiz submits one mode=text task and polls until the task goroutine
// reaches any terminal/decision state. It never sends real mail or hits the
// network: email paths fail fast on the missing AIOPS_KEY, and notes are local.
func postTaskBiz(t *testing.T, base, text string) map[string]any {
	t.Helper()
	resp := postJSON(t, base+"/v1/tasks", "", map[string]string{"text": text, "mode": "text"})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /v1/tasks 应 202, got %d", resp.StatusCode)
	}
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	_ = resp.Body.Close()
	if ack.TaskID == "" {
		t.Fatal("应返回 task_id")
	}
	var body map[string]any
	for i := 0; i < 150; i++ {
		r, err := http.Get(base + "/v1/tasks/" + ack.TaskID)
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		body = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = r.Body.Close()
		switch body["status"] {
		case "done", "failed", "need_confirm", "need_ask", "canceled", "in_progress", "interrupted", "revoked":
			return body
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("任务未到达终态: %v", body)
	return nil
}

// R-a: an external action that fails (email fetch, no AIOPS_KEY — fails BEFORE
// any HTTP request, so this test never touches the network) must NOT be reported
// as business done. Regression for the v0.8 A/B finding: FAILED email but HTTP done.
func TestBizStatus_EmailFetchFailureNotDone(t *testing.T) {
	t.Setenv("AIOPS_KEY", "")
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	body := postTaskBiz(t, ts.URL, "回复客户邮件说收到")
	if body["status"] != "failed" {
		t.Fatalf("外部取信失败时终态必须 failed，不得 done；got status=%v body=%v", body["status"], body)
	}
}

// R-b: the first turn of a high-risk install opens a confirmation gate; the task
// must park at need_confirm, never done (the v0.8 A/B found it returned done with
// only a "待确认：" receipt). The gate is produced inside execInstall before any
// node/npm check, so no host change occurs.
func TestBizStatus_InstallWaitsConfirmNotDone(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	body := postTaskBiz(t, ts.URL, "帮我安装 codex 到后台")
	if body["status"] != "need_confirm" {
		t.Fatalf("安装首次必须停在 need_confirm，不得 done；got status=%v body=%v", body["status"], body)
	}
}

// R-c: a two-step local chain must DRAIN within the same turn — both notes land
// and the durable goal completes with an empty queue. Regression for the v0.8 A/B
// finding "先查时间再记开会" where step 2 stayed parked forever yet the turn was done.
func TestBizStatus_SequenceDrainsQueueSameTurn(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	body := postTaskBiz(t, ts.URL, "先记一下队列甲再记一下队列乙")
	if body["status"] != "done" {
		t.Fatalf("两步本地任务同轮排空后应 done，got status=%v body=%v", body["status"], body)
	}
	notes, _ := os.ReadFile(filepath.Join(dir, "notes.md"))
	if !strings.Contains(string(notes), "队列甲") || !strings.Contains(string(notes), "队列乙") {
		t.Fatalf("两步 NOTE 都应在同一轮落盘，notes=%q", string(notes))
	}
	gb, err := os.ReadFile(filepath.Join(dir, "context_slots", "goal-local-device.json"))
	if err != nil {
		t.Fatalf("应持久化 goal: %v", err)
	}
	var g struct {
		Status string   `json:"status"`
		Queue  []string `json:"queue"`
	}
	if err := json.Unmarshal(gb, &g); err != nil {
		t.Fatalf("goal JSON 解析失败: %v", err)
	}
	if g.Status != "completed" || len(g.Queue) != 0 {
		t.Fatalf("队列排空后 goal 应 completed 且 queue 空，got status=%q queue=%v", g.Status, g.Queue)
	}
}

// postTaskBizWithID is postTaskBiz but also returns the task id so the consumer
// endpoints (/answer, /events) can be exercised against a settled task.
func postTaskBizWithID(t *testing.T, base, text string) (string, map[string]any) {
	t.Helper()
	resp := postJSON(t, base+"/v1/tasks", "", map[string]string{"text": text, "mode": "text"})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /v1/tasks 应 202, got %d", resp.StatusCode)
	}
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	_ = resp.Body.Close()
	if ack.TaskID == "" {
		t.Fatal("应返回 task_id")
	}
	var body map[string]any
	for i := 0; i < 150; i++ {
		r, err := http.Get(base + "/v1/tasks/" + ack.TaskID)
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		body = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = r.Body.Close()
		switch body["status"] {
		case "done", "failed", "need_confirm", "need_ask", "canceled", "in_progress", "interrupted", "revoked":
			return ack.TaskID, body
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("任务未到达终态: %v", body)
	return "", nil
}

// expectAnswerConflict locks the answer endpoint contract: answering a task that
// is not parked at a confirmation gate is rejected with 409 instead of being
// silently applied or reported as a fresh success.
func expectAnswerConflict(t *testing.T, base, id string) {
	t.Helper()
	resp := postJSON(t, base+"/v1/tasks/"+id+"/answer", "", map[string]string{"answer": "确认"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("非待确认任务 POST /answer 必须 409，got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// expectSSEClosesWith verifies that a LATE SSE subscriber to an already-settled
// task receives the terminal event and that the stream is then CLOSED (the handler
// returns). A handler that leaves such a subscriber hanging would make the iOS
// client wait forever for events that can no longer arrive.
func expectSSEClosesWith(t *testing.T, base, id, event string) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(base + "/v1/tasks/" + id + "/events")
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body) // returns on handler close; client times out if it hangs
	if !strings.Contains(string(b), "event: "+event) {
		t.Fatalf("晚连 SSE 必须重放终态事件 event: %s 并关流，实际 body=%q", event, string(b))
	}
}

// A FAILED task must reject further /answer with 409, and a late SSE subscriber
// must receive event: failed and have the stream closed (not hang).
func TestBizStatus_FailedTaskAnswer409AndSSECloses(t *testing.T) {
	t.Setenv("AIOPS_KEY", "")
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	id, body := postTaskBizWithID(t, ts.URL, "回复客户邮件说收到")
	if body["status"] != "failed" {
		t.Fatalf("前置夹具应 failed，got status=%v body=%v", body["status"], body)
	}
	expectAnswerConflict(t, ts.URL, id)
	expectSSEClosesWith(t, ts.URL, id, "failed")
}

// A DONE task must reject further /answer with 409 and close a late SSE stream
// after replaying event: done.
func TestBizStatus_DoneTaskAnswer409AndSSECloses(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	id, body := postTaskBizWithID(t, ts.URL, "记一下 消费者完成")
	if body["status"] != "done" {
		t.Fatalf("前置夹具应 done，got status=%v body=%v", body["status"], body)
	}
	expectAnswerConflict(t, ts.URL, id)
	expectSSEClosesWith(t, ts.URL, id, "done")
}

// A sequence longer than the per-turn drain cap (MaxQueuedSteps=8; the first step
// runs immediately and up to 8 more drain, so 10 steps leave 1 parked) must settle
// as in_progress — NOT done — with the residual step persisted on the active goal.
// Consumers must treat it like the other non-gate terminal states: /answer -> 409,
// and a late SSE subscriber receives event: in_progress then the stream closes.
func TestBizStatus_OverCapSequenceIsInProgressAndSSECloses(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	nums := []string{"一", "二", "三", "四", "五", "六", "七", "八", "九", "十"}
	parts := []string{"先记一下长链步" + nums[0]}
	for i := 1; i < len(nums); i++ {
		parts = append(parts, "再记一下长链步"+nums[i])
	}
	text := strings.Join(parts, "")

	id, body := postTaskBizWithID(t, ts.URL, text)
	if body["status"] != "in_progress" {
		t.Fatalf("超过单轮排空上限的长链必须 in_progress，不得 done；got status=%v body=%v", body["status"], body)
	}
	gb, err := os.ReadFile(filepath.Join(dir, "context_slots", "goal-local-device.json"))
	if err != nil {
		t.Fatalf("应持久化 active goal: %v", err)
	}
	var g struct {
		Status string   `json:"status"`
		Queue  []string `json:"queue"`
	}
	if err := json.Unmarshal(gb, &g); err != nil {
		t.Fatalf("goal JSON 解析失败: %v", err)
	}
	if g.Status != "active" || len(g.Queue) != 1 {
		t.Fatalf("长链应留 1 步在 active goal 队列，got status=%q queue=%v", g.Status, g.Queue)
	}
	expectAnswerConflict(t, ts.URL, id)
	expectSSEClosesWith(t, ts.URL, id, "in_progress")
}
