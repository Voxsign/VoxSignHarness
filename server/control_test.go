package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"voicesign-harness/config"
)

// controlface(   v1 §6): interrupt global stop/refer  disconnect, withdraw split  back. 
//      taskState status,   dependency   pipeline donetime . 

func newControlSrv(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Server.Token = "secret"
	srv := New(&cfg, testOpts(t, dir))
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/interrupt", srv.auth(srv.handleInterrupt))
	mux.HandleFunc("/v1/withdraw", srv.auth(srv.handleWithdraw))
	mux.HandleFunc("/v1/tasks", srv.auth(srv.handleTasksPost))
	return srv, httptest.NewServer(mux)
}

// postControl sendcontrolface POST( use server_test.go   postJSON,   token). 
func postControl(t *testing.T, ts *httptest.Server, path string, body any) (int, map[string]any) {
	t.Helper()
	resp := postJSON(t, ts.URL+path, "secret", body)
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func TestInterruptGlobalStopsAllActive(t *testing.T) {
	srv, ts := newControlSrv(t)
	defer ts.Close()

	//   task:    running(ctx cancel   ),    waiting_confirm  raise. 
	a := &taskState{ID: "task-a", Status: stRunning, confirmCh: make(chan bool, 1), Priority: 50}
	b := &taskState{ID: "task-b", Status: stWaiting, confirmCh: make(chan bool, 1), Priority: 80}
	a.cancel, b.cancel = func() {}, func() {}
	srv.mu.Lock()
	srv.tasks[a.ID] = a
	srv.tasks[b.ID] = b
	srv.mu.Unlock()

	status, m := postControl(t, ts, "/v1/interrupt", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("interrupt 应 200, got %d: %v", status, m)
	}
	interrupted, _ := m["interrupted"].([]any)
	if len(interrupted) != 2 {
		t.Fatalf("应打断 2 个任务, got %v", m)
	}
	if a.Status != stCanceled || b.Status != stCanceled {
		t.Fatalf("任务应标记 canceled, a=%s b=%s", a.Status, b.Status)
	}
}

func TestInterruptSpecificTask(t *testing.T) {
	srv, ts := newControlSrv(t)
	defer ts.Close()

	a := &taskState{ID: "task-a", Status: stRunning, confirmCh: make(chan bool, 1)}
	b := &taskState{ID: "task-b", Status: stRunning, confirmCh: make(chan bool, 1)}
	a.cancel = func() {}
	srv.mu.Lock()
	srv.tasks[a.ID] = a
	srv.tasks[b.ID] = b
	srv.mu.Unlock()

	status, m := postControl(t, ts, "/v1/interrupt", map[string]any{"task_id":"task-a"})
	if status != http.StatusOK {
		t.Fatalf("应 200, got %d: %v", status, m)
	}
	if a.Status != stCanceled {
		t.Fatalf("指定任务应取消, got %s", a.Status)
	}
	if b.Status != stRunning {
		t.Fatalf("未指定任务不受影响, got %s", b.Status)
	}
}

func TestInterruptUnknownTask404(t *testing.T) {
	srv, ts := newControlSrv(t)
	defer ts.Close()
	srv.mu.Lock()
	srv.tasks["task-a"] = &taskState{ID: "task-a", Status: stDone, confirmCh: make(chan bool, 1)}
	srv.mu.Unlock()

	status, m := postControl(t, ts, "/v1/interrupt", map[string]any{"task_id":"nope"})
	if status != http.StatusNotFound {
		t.Fatalf("未知 task_id 应 404, got %d: %v", status, m)
	}
}

func TestWithdrawRunningCancels(t *testing.T) {
	srv, ts := newControlSrv(t)
	defer ts.Close()

	a := &taskState{ID: "task-a", Status: stRunning, confirmCh: make(chan bool, 1)}
	a.cancel = func() {}
	srv.mu.Lock()
	srv.tasks[a.ID] = a
	srv.mu.Unlock()

	status, m := postControl(t, ts, "/v1/withdraw", map[string]any{"task_id":"task-a","scope":"running"})
	if status != http.StatusOK {
		t.Fatalf("withdraw running 应 200, got %d: %v", status, m)
	}
	if a.Status != stCanceled {
		t.Fatalf("运行中撤回应 canceled, got %s", a.Status)
	}
}

func TestWithdrawPendingCancelsHanging(t *testing.T) {
	srv, ts := newControlSrv(t)
	defer ts.Close()

	a := &taskState{ID: "task-a", Status: stNeedAsk, confirmCh: make(chan bool, 1)}
	srv.mu.Lock()
	srv.tasks[a.ID] = a
	srv.mu.Unlock()

	status, m := postControl(t, ts, "/v1/withdraw", map[string]any{"task_id":"task-a","scope":"pending"})
	if status != http.StatusOK {
		t.Fatalf("withdraw pending 应 200, got %d: %v", status, m)
	}
	if a.Status != stCanceled {
		t.Fatalf("挂起任务撤回应 canceled, got %s", a.Status)
	}
}

func TestWithdrawDoneMarksRevoked(t *testing.T) {
	srv, ts := newControlSrv(t)
	defer ts.Close()

	a := &taskState{ID: "task-a", Status: stDone, Reversible: true, confirmCh: make(chan bool, 1)}
	srv.mu.Lock()
	srv.tasks[a.ID] = a
	srv.mu.Unlock()

	status, m := postControl(t, ts, "/v1/withdraw", map[string]any{"task_id":"task-a","scope":"done"})
	if status != http.StatusOK {
		t.Fatalf("withdraw done 应 200, got %d: %v", status, m)
	}
	if a.Status != stRevoked {
		t.Fatalf("已完成撤回应标记 revoked, got %s", a.Status)
	}
	if m["revoked"] != true {
		t.Fatalf("应返回 revoked=true, got %v", m)
	}
}

func TestWithdrawBadScope400(t *testing.T) {
	srv, ts := newControlSrv(t)
	defer ts.Close()
	srv.mu.Lock()
	srv.tasks["task-a"] = &taskState{ID: "task-a", Status: stRunning, confirmCh: make(chan bool, 1)}
	srv.mu.Unlock()

	status, _ := postControl(t, ts, "/v1/withdraw", map[string]any{"task_id":"task-a","scope":"explode"})
	if status != http.StatusBadRequest {
		t.Fatalf("非法 scope 应 400, got %d", status)
	}
}

func TestPriorityClamp(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 50}, {50, 50}, {255, 255}, {999, 255}, {-5, 0},
	}
	for _, c := range cases {
		if got := clampPriority(c.in); got != c.want {
			t.Fatalf("clampPriority(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
