package server

import (
	"encoding/json"
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
