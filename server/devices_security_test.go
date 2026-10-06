package server

// devices_security_test.go —   connect preventprotectback (2026-10-05 recvtailfix ). 
//
//    docs/  close   - endserveservice end  -20261005.md  place   fix  as: 
//  ① basely form(CloudMode=false, no VHS_TOKEN): register/heartbeat/lookup   panic, returnback 501; 
//  ②  end form + VHS_TOKEN already : no  /  token -> 401, pos  Bearer token -> 200 and note table; 
//  ③  end form + VHS_TOKEN asempty: register/heartbeat -> 503( protect,   empty    ). 
//
// safety   httptest + s.Handler()    routeby  (andoccurproduce Start() same   mux). 

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"voicesign-harness/config"
)

// newDevicesSrv by CloudMode/token    Server and    Handler(). 
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

// ① basely form:   note table store , register/heartbeat/lookup    501,    panic. 
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

// ②  end form + VHS_TOKEN already : 401/401/200    boundary +        table. 
func TestDevicesCloudTokenAuth(t *testing.T) {
	srv, ts := newDevicesSrv(t, true, "secret")
	defer ts.Close()

	const mc = "MC-CLOUD-1"
	body := map[string]any{"machine_code": mc, "name": "mac", "base": "http://10.0.0.2:8897"}

	// no   -> 401
	resp := postJSON(t, ts.URL+"/v1/devices/register", "", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("register 无凭证应 401，got %d", resp.StatusCode)
	}
	// error token -> 401
	resp = postJSON(t, ts.URL+"/v1/devices/register", "wrong-token", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("register 错误 token 应 401，got %d", resp.StatusCode)
	}
	// pos  token -> 200
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
	//        table( connect note table)
	srv.devices.mu.Lock()
	rec, ok := srv.devices.devices[mc]
	srv.devices.mu.Unlock()
	if !ok || rec == nil || !rec.Online {
		t.Fatalf("设备记录应已创建且 online，got rec=%+v ok=%v", rec, ok)
	}

	// heartbeat: no   -> 401; pos  token -> 200
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

// ③  end form + VHS_TOKEN asempty:  protect register/heartbeat -> 503( again   200). 
// lookup basethen   (  codei.e.  ),    protect  . 
func TestDevicesCloudEmptyTokenHardBlock(t *testing.T) {
	srv, ts := newDevicesSrv(t, true, "")
	defer ts.Close()

	if srv.devices == nil {
		t.Fatal("云端模式 s.devices 应已创建（前置假设）")
	}
	for _, p := range []string{"/v1/devices/register", "/v1/devices/heartbeat"} {
		//    ( empty)  all be 503  under: serveserviceend   token, clientuserendno token   
		resp := postJSON(t, ts.URL+p, "whatever", map[string]any{"machine_code": "MC-1"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("云端漏配 VHS_TOKEN 时 %s 应 503（硬保护），got %d", p, resp.StatusCode)
		}
	}
	//  protectoccur : note table  bewrite
	if n := len(srv.devices.devices); n != 0 {
		t.Fatalf("503 拦截下注册表应保持空表，got %d 条", n)
	}
}
