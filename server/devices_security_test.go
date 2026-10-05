package server

// devices_security_test.go — 设备接口防护回归（2026-10-05 收尾修复）。
//
// 锁定 docs/测试结果报告-云端服务器端功能-20261005.md 三处问题的修复行为：
//  ① 本地模式（CloudMode=false、无 VHS_TOKEN）：register/heartbeat/lookup 不 panic，返回 501；
//  ② 云端模式 + VHS_TOKEN 已设：无凭证/错 token → 401，正确 Bearer token → 200 且落注册表；
//  ③ 云端模式 + VHS_TOKEN 为空：register/heartbeat → 503（硬保护，堵住空凭证放行）。
//
// 全部经 httptest + s.Handler() 走真实路由装配（与生产 Start() 同一份 mux）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"voicesign-harness/config"
)

// newDevicesSrv 按 CloudMode/token 构造 Server 并挂真实 Handler()。
func newDevicesSrv(t *testing.T, cloudMode bool, token string) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Global.CloudMode = cloudMode
	cfg.Server.Token = token
	srv := New(&cfg, testOpts(t, dir))
	return srv, httptest.NewServer(srv.Handler())
}

// ① 本地模式：设备注册表不存在，register/heartbeat/lookup 一律 501，绝不 panic。
func TestDevicesLocalModeGuard(t *testing.T) {
	srv, ts := newDevicesSrv(t, false, "")
	defer ts.Close()

	if srv.devices != nil {
		t.Fatal("本地模式 s.devices 应为 nil（前置假设）")
	}
	for _, p := range []string{"/v1/devices/register", "/v1/devices/heartbeat", "/v1/devices/lookup"} {
		resp := postJSON(t, ts.URL+p, "", map[string]any{"machine_code": "MC-LOCAL-1"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Fatalf("本地模式 %s 应 501（不 panic），got %d", p, resp.StatusCode)
		}
	}
}

// ② 云端模式 + VHS_TOKEN 已设：401/401/200 鉴权边界 + 设备记录真实落表。
func TestDevicesCloudTokenAuth(t *testing.T) {
	srv, ts := newDevicesSrv(t, true, "secret")
	defer ts.Close()

	const mc = "MC-CLOUD-1"
	body := map[string]any{"machine_code": mc, "name": "mac", "base": "http://10.0.0.2:8897"}

	// 无凭证 → 401
	resp := postJSON(t, ts.URL+"/v1/devices/register", "", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("register 无凭证应 401，got %d", resp.StatusCode)
	}
	// 错误 token → 401
	resp = postJSON(t, ts.URL+"/v1/devices/register", "wrong-token", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("register 错误 token 应 401，got %d", resp.StatusCode)
	}
	// 正确 token → 200
	resp = postJSON(t, ts.URL+"/v1/devices/register", "secret", body)
	var reg struct {
		OK          bool   `json:"ok"`
		MachineCode string `json:"machine_code"`
		Token       string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !reg.OK || reg.Token == "" {
		t.Fatalf("register 正确 token 应 200 且签发设备 token，got %d %+v", resp.StatusCode, reg)
	}
	// 设备记录真实落表（直接查注册表）
	srv.devices.mu.Lock()
	rec, ok := srv.devices.devices[mc]
	srv.devices.mu.Unlock()
	if !ok || rec == nil || !rec.Online {
		t.Fatalf("设备记录应已创建且 online，got rec=%+v ok=%v", rec, ok)
	}

	// heartbeat：无凭证 → 401；正确 token → 200
	resp = postJSON(t, ts.URL+"/v1/devices/heartbeat", "", map[string]any{"machine_code": mc, "state": "idle"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("heartbeat 无凭证应 401，got %d", resp.StatusCode)
	}
	resp = postJSON(t, ts.URL+"/v1/devices/heartbeat", "secret", map[string]any{"machine_code": mc, "state": "decision", "pending": 1})
	var hb struct {
		OK    bool   `json:"ok"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hb); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !hb.OK || hb.State != "decision" {
		t.Fatalf("heartbeat 正确 token 应 200 且 state=decision，got %d %+v", resp.StatusCode, hb)
	}
}

// ③ 云端模式 + VHS_TOKEN 为空：硬保护 register/heartbeat → 503（不再放行 200）。
// lookup 本就免鉴权（机器码即凭证），不在硬保护范围。
func TestDevicesCloudEmptyTokenHardBlock(t *testing.T) {
	srv, ts := newDevicesSrv(t, true, "")
	defer ts.Close()

	if srv.devices == nil {
		t.Fatal("云端模式 s.devices 应已创建（前置假设）")
	}
	for _, p := range []string{"/v1/devices/register", "/v1/devices/heartbeat"} {
		// 带任意（含空）凭证都应被 503 拦下：服务端没配 token，客户端无 token 可带
		resp := postJSON(t, ts.URL+p, "whatever", map[string]any{"machine_code": "MC-1"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("云端漏配 VHS_TOKEN 时 %s 应 503（硬保护），got %d", p, resp.StatusCode)
		}
	}
	// 硬保护生效：注册表不得被写入
	if n := len(srv.devices.devices); n != 0 {
		t.Fatalf("503 拦截下注册表应保持空表，got %d 条", n)
	}
}
