package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"voicesign-harness/config"
	"testing"
	"time"
)

// 自适应心跳（架构 v1 §5）：服务端窗口化在线判定 + heartbeat 状态/待办更新。

func TestDeviceOnlineWindowed(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		state    string
		lastSeen time.Time
		want     bool
	}{
		{"decision 刚心跳", "decision", now.Add(-2 * time.Second), true},
		{"decision 超过 3×5s", "decision", now.Add(-20 * time.Second), false},
		{"busy 刚心跳", "busy", now.Add(-10 * time.Second), true},
		{"idle 2min 内", "idle", now.Add(-3 * time.Minute), true},
		{"idle 超过 3×2min", "idle", now.Add(-7 * time.Minute), false},
		{"standby 5min 内", "standby", now.Add(-10 * time.Minute), true},
		{"standby 超过 3×5min", "standby", now.Add(-16 * time.Minute), false},
		{"从未心跳", "", time.Time{}, false},
	}
	for _, c := range cases {
		rec := &DeviceRecord{State: c.state, LastSeen: c.lastSeen}
		if got := deviceOnline(rec); got != c.want {
			t.Fatalf("%s: deviceOnline = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHeartbeatPeriod(t *testing.T) {
	cases := []struct{ state string; want time.Duration }{
		{"decision", 5 * time.Second},
		{"busy", 15 * time.Second},
		{"idle", 2 * time.Minute},
		{"standby", 5 * time.Minute},
		{"unknown", 2 * time.Minute}, // 未知状态回落 idle
	}
	for _, c := range cases {
		if got := heartbeatPeriod(c.state); got != c.want {
			t.Fatalf("heartbeatPeriod(%q) = %v, want %v", c.state, got, c.want)
		}
	}
}

func TestHeartbeatUpdatesState(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Global.CloudMode = true // devices 注册表仅在云端模式创建
	cfg.Server.Token = "secret" // register/heartbeat 需 Bearer VHS_TOKEN
	srv := New(&cfg, testOpts(t, dir))
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/devices/register", srv.deviceToken(srv.handleDevicesRegister))
	mux.HandleFunc("/v1/devices/heartbeat", srv.deviceToken(srv.handleDevicesHeartbeat))
	mux.HandleFunc("/v1/devices/lookup", srv.public(srv.handleDevicesLookup))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 注册设备（模拟 harness 注册）。
	status, _ := postControl(t, ts, "/v1/devices/register",
		map[string]any{"machine_code": "ABCD-EFGH-JKLM", "name": "mac", "base": "http://10.0.0.2:8897"})
	if status != http.StatusOK {
		t.Fatalf("注册应 200, got %d", status)
	}

	// 心跳带 state/pending。
	status, m := postControl(t, ts, "/v1/devices/heartbeat",
		map[string]any{"machine_code": "ABCD-EFGH-JKLM", "state": "decision", "pending": 1})
	if status != http.StatusOK {
		t.Fatalf("心跳应 200, got %d: %v", status, m)
	}

	// lookup 返回窗口化 online + state + pending。
	resp := postJSON(t, ts.URL+"/v1/devices/lookup", "secret",
		map[string]any{"machine_code": "ABCD-EFGH-JKLM"})
	defer resp.Body.Close()
	var lk struct {
		Online bool   `json:"online"`
		State  string `json:"state"`
		Pending int   `json:"pending"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lk); err != nil {
		t.Fatal(err)
	}
	if !lk.Online || lk.State != "decision" || lk.Pending != 1 {
		t.Fatalf("lookup 应 online+decision+pending=1, got %+v", lk)
	}
}

func TestActivityState(t *testing.T) {
	srv, _ := newControlSrv(t)
	srv.tasks = map[string]*taskState{} // 清空恢复表，保证空表基线

	// 空任务表 → idle。
	if st, p, _ := srv.ActivityState(); st != "idle" || p != 0 {
		t.Fatalf("空表应 idle/0, got %s/%d", st, p)
	}
	srv.mu.Lock()
	srv.tasks["a"] = &taskState{ID: "a", Status: stRunning, confirmCh: make(chan bool, 1)}
	srv.tasks["b"] = &taskState{ID: "b", Status: stRunning, confirmCh: make(chan bool, 1)}
	srv.mu.Unlock()
	if st, p, _ := srv.ActivityState(); st != "busy" || p != 2 {
		t.Fatalf("两个 running 应 busy/2, got %s/%d", st, p)
	}
	srv.mu.Lock()
	srv.tasks["c"] = &taskState{ID: "c", Status: stNeedConfirm, confirmCh: make(chan bool, 1)}
	srv.mu.Unlock()
	if st, p, _ := srv.ActivityState(); st != "decision" || p != 1 {
		t.Fatalf("存在 need_confirm 应 decision/1, got %s/%d", st, p)
	}
}
