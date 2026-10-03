package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/memory"
	"voicesign-harness/pipeline"
	"voicesign-harness/refer"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
)

func testOpts(t *testing.T, logDir string) *pipeline.Options {
	t.Helper()
	cfg := config.Default()
	cfg.Global.LogDir = logDir
	cfg.Memory.Dir = filepath.Join(logDir, "mem")
	cfg.Spaces.Dir = filepath.Join(logDir, "spaces")
	cfg.Contracts.Dir = filepath.Join(logDir, "contracts")
	cfg.Cache.Dir = filepath.Join(logDir, "cache")
	for _, d := range []string{cfg.Memory.Dir, cfg.Spaces.Dir, cfg.Contracts.Dir, cfg.Cache.Dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dict, _ := memory.LoadDictionary(filepath.Join(cfg.Memory.Dir, "dictionary.json"))
	spaces, _ := space.Load(cfg.Spaces.Dir)
	reg, _ := tools.LoadContracts(cfg.Contracts.Dir)
	tr, _ := trajectory.Open(cfg.Global.LogDir)
	st, _ := cache.Open(filepath.Join(cfg.Cache.Dir, "quad.json"), time.Hour)
	return &pipeline.Options{
		Cfg:       &cfg,
		Dict:      dict,
		Spaces:    spaces,
		Refer:     refer.New(dict),
		Cache:     st,
		Tools:     reg,
		Exec:      &tools.Executor{BaseDir: logDir},
		Verifier:  &verify.Verifier{BaseDir: logDir},
		ConfirmFn: func(string, string) (bool, error) { return true, nil },
		Trace:     tr,
	}
}

func TestServerHealthAndAuth(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))

	ts := httptest.NewServer((func() http.Handler {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/health", srv.auth(srv.handleHealth))
		return mux
	})())
	defer ts.Close()

	// 无 token → 401
	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401, got %d", resp.StatusCode)
	}

	// 带 token → 200
	req, _ := http.NewRequest("GET", ts.URL+"/v1/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("带 token 应 200, got %d", resp2.StatusCode)
	}
}

func TestServerRunAndTaskGet(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))

	ts := httptest.NewServer((func() http.Handler {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/run", srv.auth(srv.handleRun))
		mux.HandleFunc("/v1/task/", srv.auth(srv.handleTaskGet))
		mux.HandleFunc("/v1/health", srv.auth(srv.handleHealth))
		return mux
	})())
	defer ts.Close()

	body, _ := json.Marshal(runReq{Text: "记一下服务器测试想法"})
	resp, err := http.Post(ts.URL+"/v1/run", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("run 应 202, got %d", resp.StatusCode)
	}
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	if ack.TaskID == "" {
		t.Fatal("应返回 task_id")
	}

	// 轮询直到 done
	var state taskState
	for i := 0; i < 50; i++ {
		r, _ := http.Get(ts.URL + "/v1/task/" + ack.TaskID)
		_ = json.NewDecoder(r.Body).Decode(&state)
		if state.Status == stDone || state.Status == stCanceled {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if state.Status != stDone {
		t.Fatalf("任务应 done, got %s: %+v", state.Status, state)
	}
	if state.Outcome == nil || state.Outcome.View.Action == "" {
		t.Fatal("应返回 Outcome 四行视图")
	}
	fmt.Printf("view=%+v\n", state.Outcome.View)
}

// muxV1 注册 INTERACT-v1 全部正式端点（测试用）。
func muxV1(srv *Server) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tasks", srv.auth(srv.handleTasksPost))
	mux.HandleFunc("/v1/tasks/", srv.auth(srv.handleTasksSub))
	mux.HandleFunc("/v1/status", srv.auth(srv.handleStatus))
	mux.HandleFunc("/v1/roles", srv.auth(srv.handleRoles))
	return httptest.NewServer(mux)
}

func postJSON(t *testing.T, url, token string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestTasksLifecycleConfirm：POST /v1/tasks → need_confirm → answer 执行 → done。
func TestTasksLifecycleConfirm(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	// 注册 proj 项目域让 COMMIT 过 space_check
	_ = srv.tmpl.Spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{dir},
		Tools: []string{"git", "read"}, Perms: space.Perms{Read: true, Write: true},
	})
	ts := muxV1(srv)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/v1/tasks", "", map[string]string{"text": "在 proj 提交所有改动"})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /v1/tasks 应 202, got %d", resp.StatusCode)
	}
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)

	// 轮询到 need_confirm
	var body map[string]any
	for i := 0; i < 50; i++ {
		r, _ := http.Get(ts.URL + "/v1/tasks/" + ack.TaskID)
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["status"] == "need_confirm" || body["status"] == "done" || body["status"] == "canceled" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if body["status"] != "need_confirm" {
		t.Fatalf("COMMIT 应 need_confirm, got %+v", body)
	}

	// answer 确认词
	ans := postJSON(t, ts.URL+"/v1/tasks/"+ack.TaskID+"/answer", "", map[string]string{"answer": "执行"})
	if ans.StatusCode != http.StatusOK {
		t.Fatalf("answer 应 200, got %d", ans.StatusCode)
	}
	// 轮询到 done
	for i := 0; i < 50; i++ {
		r, _ := http.Get(ts.URL + "/v1/tasks/" + ack.TaskID)
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["status"] == "done" || body["status"] == "canceled" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if body["status"] != "done" {
		t.Fatalf("放行后应 done, got %+v", body)
	}
	if _, ok := body["receipt"]; !ok {
		t.Fatalf("done 应带 receipt 四行: %+v", body)
	}
}

// TestAnswerNoPendingConflict：done 后再 answer → 409。
func TestAnswerNoPendingConflict(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/v1/tasks", "", map[string]string{"text": "记一下测试"})
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	var body map[string]any
	for i := 0; i < 50; i++ {
		r, _ := http.Get(ts.URL + "/v1/tasks/" + ack.TaskID)
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["status"] == "done" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	ans := postJSON(t, ts.URL+"/v1/tasks/"+ack.TaskID+"/answer", "", map[string]string{"answer": "y"})
	if ans.StatusCode != http.StatusConflict {
		t.Fatalf("无决策点 answer 应 409, got %d", ans.StatusCode)
	}
}

// TestRollbackNote：NOTE 追加→备份→rollback 还原 notes.md。
func TestRollbackNote(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	notesPath := filepath.Join(dir, "notes.md")
	_ = os.WriteFile(notesPath, []byte("# 原始笔记\n"), 0o644)

	resp := postJSON(t, ts.URL+"/v1/tasks", "", map[string]string{"text": "记一下回滚测试"})
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	var body map[string]any
	for i := 0; i < 50; i++ {
		r, _ := http.Get(ts.URL + "/v1/tasks/" + ack.TaskID)
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["status"] == "done" || body["status"] == "canceled" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if body["status"] != "done" {
		t.Fatalf("NOTE 应 done, got %+v", body)
	}

	// rollback
	rb := postJSON(t, ts.URL+"/v1/tasks/"+ack.TaskID+"/rollback", "", map[string]string{})
	if rb.StatusCode != http.StatusOK {
		t.Fatalf("NOTE rollback 应 200, got %d", rb.StatusCode)
	}
	got, _ := os.ReadFile(notesPath)
	if string(got) != "# 原始笔记\n" {
		t.Fatalf("rollback 应还原原始内容, got %q", string(got))
	}
}

// TestRollbackIrreversible409：COMMIT 不可逆 → 409。
func TestRollbackIrreversible409(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	_ = srv.tmpl.Spaces.Add(&space.Manifest{
		Name: "proj", Type: space.TypeProject, Scope: []string{dir},
		Tools: []string{"git", "read"}, Perms: space.Perms{Read: true, Write: true},
	})
	ts := muxV1(srv)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/v1/tasks", "", map[string]string{"text": "在 proj 提交所有改动"})
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	var body map[string]any
	for i := 0; i < 50; i++ {
		r, _ := http.Get(ts.URL + "/v1/tasks/" + ack.TaskID)
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["status"] == "need_confirm" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 不 answer，直接 rollback（任务未 done）
	rb := postJSON(t, ts.URL+"/v1/tasks/"+ack.TaskID+"/rollback", "", map[string]string{})
	if rb.StatusCode != http.StatusConflict {
		t.Fatalf("未完成任务 rollback 应 409, got %d", rb.StatusCode)
	}
}

// TestStatusEndpoint：GET /v1/status 返回 version/uptime/ok。
func TestStatusEndpoint(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	// 缺 token 401
	r1, _ := http.Get(ts.URL + "/v1/status")
	if r1.StatusCode != http.StatusUnauthorized {
		t.Fatalf("缺 token 应 401, got %d", r1.StatusCode)
	}
	// 带 token 200
	req, _ := http.NewRequest("GET", ts.URL+"/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	r2, _ := http.DefaultClient.Do(req)
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("带 token 应 200, got %d", r2.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(r2.Body).Decode(&out)
	if out["ok"] != true || out["version"] == nil || out["uptime"] == nil {
		t.Fatalf("status 缺字段: %+v", out)
	}
}

// TestRequestIDDedup（M4-1 ①）：同 request_id 重复 POST 返回既有任务，不重复执行。
func TestRequestIDDedup(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	body := map[string]string{"text": "记一下去重测试", "request_id": "req-abc-123"}
	r1 := postJSON(t, ts.URL+"/v1/tasks", "", body)
	if r1.StatusCode != http.StatusAccepted {
		t.Fatalf("首次应 202, got %d", r1.StatusCode)
	}
	var ack1 struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(r1.Body).Decode(&ack1)

	// 等任务 done。
	//
	// ⚠️ 2026-10-03（`timing_sensitive_scan.py` 判 B 类 · §7"删掉跑 N 次"验证）：
	//   原为 `time.Sleep(200ms)` —— **固定等待**。**删掉它跑 1 次即失败**：
	//     `TempDir RemoveAll cleanup: unlinkat …/001: **directory not empty**`
	//   ⇒ 即：任务**仍在写盘**时测试就结束了 ⇒ 与 `t.TempDir()` 清理**竞争**
	//   ⇒ ⚠️ 而本地跑 100 次全过（200ms 够）⇒ **失败率 <1%** ⇒
	//      **但慢 CI 上 200ms 可能不够 ⇒ 同一失败会出现**
	//   ⇒ 改为**轮询到终态**（直接读 `srv.tasks` ⇒ 不走端点 ⇒ 无 mux 依赖）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		got := srv.tasks[ack1.TaskID]
		done := got != nil && (got.Status == stDone || got.Status == stCanceled)
		srv.mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 同 request_id 再提交
	r2 := postJSON(t, ts.URL+"/v1/tasks", "", body)
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("重复提交应 200（去重）, got %d", r2.StatusCode)
	}
	var ack2 struct {
		TaskID  string `json:"task_id"`
		Deduped string `json:"deduped"`
	}
	_ = json.NewDecoder(r2.Body).Decode(&ack2)
	if ack2.TaskID != ack1.TaskID {
		t.Fatalf("去重应返回同一 task_id: %s vs %s", ack1.TaskID, ack2.TaskID)
	}
	if ack2.Deduped != "true" {
		t.Fatalf("应标 deduped=true, got %q", ack2.Deduped)
	}
}

// TestTaskPersistenceRestore（M4-1 ③）：落盘 → 新 Server 恢复 → 历史可查、未完成标 interrupted。
func TestTaskPersistenceRestore(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv1 := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv1)
	resp := postJSON(t, ts.URL+"/v1/tasks", "", map[string]string{"text": "记一下持久化测试"})
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	// ⚠️ 2026-10-03（`timing_sensitive_scan.py` 判 B 类 + §7"删掉跑 N 次"验证）：
	//   原为 `time.Sleep(200ms)` —— **固定等待**。删掉它跑 10 次 ⇒ **失败**：
	//     `server_test.go:423: done 任务恢复后应保持 done, got "interrupted"`
	//   ⇒ 即：任务还在跑时就 `ts.Close()` ⇒ 盘上记 `interrupted`（那是**正确行为**）
	//   ⇒ ⚠️ 慢 CI 上 200ms 可能不够 ⇒ **同一失败会出现**，且信息**误导性强**
	//      （它说"done 应保持 done"，真实原因是"任务根本没跑完"）
	//   ⇒ 改为**轮询到终态再关闭**。
	// ⚠️ 端点必须是 **`/v1/tasks/`（复数）** —— `muxV1` 只注册了 4 条路由：
	//      `/v1/tasks` · `/v1/tasks/` · `/v1/status` · `/v1/roles`
	//    **没有 `/v1/task/`（单数）** ⇒ 我第一次改时抄了 `:130` 的单数端点 ⇒ **5 次全 404**
	//    ⇒ 轮询永远看不到 done ⇒ **修复反而把测试改坏了**（已回滚后重改）。
	var state taskState
	for i := 0; i < 250; i++ { // 250 × 20ms = **5s 上限**
		r, err := http.Get(ts.URL + "/v1/tasks/" + ack.TaskID)
		if err != nil {
			break
		}
		_ = json.NewDecoder(r.Body).Decode(&state)
		_ = r.Body.Close()
		if state.Status == stDone || state.Status == stCanceled {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if state.Status != stDone {
		t.Fatalf("持久化测试的前置条件未满足：任务应 done，实际 %q（等最多 5s）", state.Status)
	}
	ts.Close()

	// 新 Server（同一 log_dir）恢复历史任务
	srv2 := New(&cfg, testOpts(t, dir))
	if _, ok := srv2.tasks[ack.TaskID]; !ok {
		t.Fatalf("重启后应恢复任务 %s", ack.TaskID)
	}
	got := srv2.tasks[ack.TaskID]
	if got.Status != stDone {
		t.Fatalf("done 任务恢复后应保持 done, got %q", got.Status)
	}
	if got.BackupPath == "" && got.TargetPath == "" {
		// NOTE 应有 notes.md 目标
		t.Logf("warn: 无 rollback 元数据")
	}
}

// TestAskResumeViaAnswer（M4-1 ②）：need_ask → answer 澄清 → 续跑到 done。
func TestAskResumeViaAnswer(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	// 触发回问：省略域的写意图（落 global 只读 → space_check 拒绝而非 ask；
	// 用低置信 query 文本触发 ask 出口较难，直接验证状态机：need_ask 时 answer 不 409）。
	// 先造一个 need_ask 状态任务（直接构造，绕过 pipeline 不确定性）。
	srv.mu.Lock()
	fake := &taskState{
		ID: "task-fake-ask", Status: stNeedAsk, Text: "那个东西",
		Question: "你指哪个？", Options: []pipeline.AskOption{{ID: "o1", Label: "notes"}},
		confirmCh: make(chan bool, 1),
	}
	srv.tasks[fake.ID] = fake
	srv.mu.Unlock()

	r, _ := http.Post(ts.URL+"/v1/tasks/task-fake-ask/answer", "application/json",
		bytes.NewReader([]byte(`{"answer":"notes"}`)))
	if r.StatusCode != http.StatusOK {
		t.Fatalf("need_ask answer 应 200, got %d", r.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body["resumed"] != true {
		t.Fatalf("应返回 resumed=true: %+v", body)
	}
	// 续跑已驱动：pipeline 跑完后 Outcome 非空（澄清后仍歧义会再次 need_ask，属正常语义）。
	//
	// ⚠️ 2026-10-03（`timing_sensitive_scan.py` 判 B 类 · 与 `:414` 同族）：
	//   原为 `time.Sleep(200ms)` —— **固定等待**。慢 CI 上 200ms 可能不够
	//   ⇒ `t.Fatal("answer 未驱动续跑（Outcome 仍空）")` ⇒ **失败信息误导**
	//     （它说"未驱动续跑"，真实原因是"还没跑完"）
	//   ⇒ 改为**轮询到条件成立 + 明确超时**（直接读 `srv.tasks`，**不需要端点** ⇒ 无 mux 依赖）
	var got *taskState
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		got = srv.tasks["task-fake-ask"]
		done := got != nil && got.Outcome != nil
		srv.mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got == nil || got.Outcome == nil {
		t.Fatal("answer 未驱动续跑：等最多 5s 后 Outcome 仍空")
	}
}

// TestAskAnswerByOptionID（M4-3 ①）：answer 传候选 id → 续跑（prefix 强关键词驱动）。
func TestAskAnswerByOptionID(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	srv.mu.Lock()
	fake := &taskState{
		ID: "task-opt-id", Status: stNeedAsk, Text: "那个",
		Question:  "你想干嘛？",
		Options:   []pipeline.AskOption{{ID: "note", Label: "记想法"}, {ID: "edit", Label: "改文件"}},
		confirmCh: make(chan bool, 1),
	}
	srv.tasks[fake.ID] = fake
	srv.mu.Unlock()

	r, _ := http.Post(ts.URL+"/v1/tasks/task-opt-id/answer", "application/json",
		bytes.NewReader([]byte(`{"answer":"note"}`)))
	if r.StatusCode != http.StatusOK {
		t.Fatalf("answer 候选 id 应 200, got %d", r.StatusCode)
	}
	// ⚠️ 2026-10-03（`timing_sensitive_scan.py` 判 B 类 · 与 `:414`/`:497` 同族）：
	//   原为 `time.Sleep(200ms)` —— **固定等待** ⇒ 慢 CI 上可能不够 ⇒ 失败信息误导
	//   ⇒ 改为**轮询到条件成立 + 明确超时**（直接读 `srv.tasks`，**不走端点** ⇒ 无 mux 依赖）
	var got *taskState
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		got = srv.tasks["task-opt-id"]
		done := got != nil && got.Outcome != nil
		srv.mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got == nil || got.Outcome == nil {
		t.Fatal("answer=note 应驱动续跑并产出 Outcome：等最多 5s 后仍为空")
	}
	if got.Outcome.Intent.Intent != contract.IntentNote {
		t.Fatalf("answer=note 应映射为 NOTE 意图, got %q", got.Outcome.Intent.Intent)
	}
}

// TestRolesEndpoint（M5-3）：/v1/roles 返回三角色 + 当前任务角色。
func TestRolesEndpoint(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()
	get := func(path string) map[string]any {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer secret")
		r, _ := http.DefaultClient.Do(req)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		return body
	}

	// 无任务 → 无 role 字段
	body := get("/v1/roles")
	roles, _ := body["roles"].([]any)
	if len(roles) != 3 {
		t.Fatalf("应返回 3 个角色, got %d: %+v", len(roles), body)
	}
	if _, ok := body["role"]; ok {
		t.Fatal("无任务时不应返回 role")
	}

	// 造一个 need_confirm 任务
	srv.mu.Lock()
	fake := &taskState{
		ID: "task-role-1", Status: stNeedConfirm, Role: RolePlanner,
		confirmCh: make(chan bool, 1),
	}
	srv.tasks[fake.ID] = fake
	srv.mu.Unlock()

	body2 := get("/v1/roles")
	if body2["role"] != RolePlanner {
		t.Fatalf("need_confirm 任务 role 应为 planner, got %v", body2["role"])
	}
}

// TestTaskStatusHasRole（M5-3）：任务状态响应含合法 role。
func TestTaskStatusHasRole(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	srv.mu.Lock()
	fake := &taskState{ID: "t-done", Status: stDone, Role: RoleVerifier, confirmCh: make(chan bool, 1)}
	srv.tasks[fake.ID] = fake
	srv.mu.Unlock()

	req, _ := http.NewRequest("GET", ts.URL+"/v1/tasks/t-done", nil)
	req.Header.Set("Authorization", "Bearer secret")
	r, _ := http.DefaultClient.Do(req)
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body["role"] != RoleVerifier {
		t.Fatalf("done 任务 role 应为 verifier, got %v", body["role"])
	}
}

// readSSE 从 resp body 解析事件序列（直到 done/canceled 或超时）。
func readSSE(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out []map[string]any
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev map[string]any
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev) == nil {
			out = append(out, ev)
			if typ, _ := ev["type"].(string); typ == "done" || typ == "canceled" || typ == "failed" {
				break
			}
		}
	}
	return out
}

func getSSE(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestSSEEventSequence（M6-1）：NOTE 任务 SSE 序列有序、seq 递增、终态 done。
func TestSSEEventSequence(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/v1/tasks", "secret", map[string]string{"text": "记一下 sse 测试"})
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)

	r := getSSE(t, ts.URL+"/v1/tasks/"+ack.TaskID+"/events", "secret")
	events := readSSE(t, r)
	if len(events) < 2 {
		t.Fatalf("应至少 2 个事件, got %d: %+v", len(events), events)
	}
	// seq 递增
	prev := 0
	for _, ev := range events {
		seq, _ := ev["seq"].(float64)
		if int(seq) <= prev {
			t.Fatalf("seq 应递增: %v", events)
		}
		prev = int(seq)
	}
	// 终态含 done
	last := events[len(events)-1]
	if last["type"] != "done" {
		t.Fatalf("终态应为 done, got %v", last["type"])
	}
}

// TestSSEReconnectIdempotent（M6-1）：?after 重连只收其后事件。
func TestSSEReconnectIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/v1/tasks", "secret", map[string]string{"text": "记一下重连"})
	var ack struct {
		TaskID string `json:"task_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)

	// 等任务完成
	time.Sleep(300 * time.Millisecond)

	// 重连 after=1，应只看到 seq>1 的事件
	r := getSSE(t, ts.URL+"/v1/tasks/"+ack.TaskID+"/events?after=1", "secret")
	events := readSSE(t, r)
	for _, ev := range events {
		seq, _ := ev["seq"].(float64)
		if int(seq) <= 1 {
			t.Fatalf("after=1 应只收 seq>1, got %v", ev)
		}
	}
}

// TestSSENeedAskDecisionPoint（M6-1）：need_ask 事件到达。
func TestSSENeedAskDecisionPoint(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	srv.mu.Lock()
	fake := &taskState{
		ID: "task-sse-ask", Status: stNeedAsk, Text: "那个",
		Question:  "你指哪个？",
		Options:   []pipeline.AskOption{{ID: "note", Label: "记一下"}},
		confirmCh: make(chan bool, 1),
	}
	srv.tasks[fake.ID] = fake
	srv.emitEvent(fake, "need_ask", map[string]any{"question": fake.Question, "options": fake.Options})
	srv.emitEvent(fake, "done", map[string]any{"receipt": "OK"})
	srv.mu.Unlock()

	r := getSSE(t, ts.URL+"/v1/tasks/task-sse-ask/events", "secret")
	events := readSSE(t, r)
	foundAsk := false
	for _, ev := range events {
		if ev["type"] == "need_ask" {
			foundAsk = true
			if ev["question"] != "你指哪个？" {
				t.Fatalf("need_ask question 不匹配: %v", ev)
			}
		}
	}
	if !foundAsk {
		t.Fatalf("应含 need_ask 事件: %+v", events)
	}
}

// TestSSEInterruptImmediacy（M6-1）：cancel 后 interrupt 事件送达。
func TestSSEInterruptImmediacy(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	srv.mu.Lock()
	fake := &taskState{
		ID: "task-sse-cancel", Status: stRunning,
		confirmCh: make(chan bool, 1),
	}
	fake.cancel = func() {}
	srv.tasks[fake.ID] = fake
	srv.mu.Unlock()

	// 后台 cancel
	go func() {
		time.Sleep(100 * time.Millisecond)
		postJSON(t, ts.URL+"/v1/tasks/task-sse-cancel/cancel", "secret", map[string]string{})
	}()

	r := getSSE(t, ts.URL+"/v1/tasks/task-sse-cancel/events", "secret")
	events := readSSE(t, r)
	foundInterrupt := false
	for _, ev := range events {
		if ev["type"] == "interrupt" {
			foundInterrupt = true
		}
	}
	if !foundInterrupt {
		t.Fatalf("应含 interrupt 事件: %+v", events)
	}
}

// TestCORSOriginAllowed（M7）：带 Origin 请求响应回显 ACAO。
func TestCORSOriginAllowed(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Origin", "http://localhost:8080")
	r, _ := http.DefaultClient.Do(req)
	if got := r.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:8080" {
		t.Fatalf("ACAO 应回显 Origin, got %q", got)
	}
}

// TestCORSPreflight（M7）：OPTIONS 预检 204 + 头。
func TestCORSPreflight(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	req, _ := http.NewRequest("OPTIONS", ts.URL+"/v1/tasks", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	r, _ := http.DefaultClient.Do(req)
	if r.StatusCode != http.StatusNoContent {
		t.Fatalf("预检应 204, got %d", r.StatusCode)
	}
	if r.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("应含 Allow-Headers")
	}
	if r.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("应含 Allow-Methods")
	}
}

// TestCORSNoOriginUnaffected（M7）：无 Origin 不加头、401 行为不回归。
func TestCORSNoOriginUnaffected(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	ts := muxV1(srv)
	defer ts.Close()

	r, _ := http.Get(ts.URL + "/v1/status")
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401, got %d", r.StatusCode)
	}
	if r.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("无 Origin 请求不应加 ACAO 头")
	}
}
