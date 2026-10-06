//go:build vhs002

// restart_criteria_test.go -- SCOPE-REF-02  ** as data**: serveservice nostatus. 
//
//  code  close " hasglobal map"* is* data; **heavystartafter howeverclarification**onlyis. 
//    restrict:   confirmed ->  process -> heavystart(same  dataDir)->    confirmed ⇒   clarification. 
package asr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return fmt.Sprintf("127.0.0.1:%d", l.Addr().(*net.TCPAddr).Port)
}

func startRealServer(t *testing.T, bin, addr, dataDir string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "VHS_ASR_ADDR="+addr, "VHS_ASR_DATA="+dataDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动真进程失败: %v", err)
	}
	base := "http://" + addr
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(base + "/v1/health"); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return cmd
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	t.Fatal("真进程未在 10s 内就绪")
	return nil
}

func postProcess(t *testing.T, base, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(base+"/v1/process", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSCOPEREF02StatelessAcrossRestart(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "vhs-asr")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vhs-asr")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("编译真二进制失败: %v\n%s", err, out)
	}
	dataDir := t.TempDir() // **same  dataDir  heavystart**: ifserveservicepipe  state  ,   then  use
	addr := freePort(t)

	//    : confirm require ⇒  to   close 
	cmd1 := startRealServer(t, bin, addr, dataDir)
	first := postProcess(t, "http://"+addr, `{"text":"确认，就是报价模块","session_id":"restart-probe"}`)
	conf, ok := first["confirmable"].(map[string]any)
	if !ok || conf["canonical"] == nil {
		_ = cmd1.Process.Kill()
		t.Fatalf("[重启] 第一次未返回可携带确认结构: %v", first)
	}
	canon, _ := conf["canonical"].(string)

	//  back confirmed  sameprocess require ⇒  use(baseline)
	got := postProcess(t, "http://"+addr, `{"text":"把那个模块改了","session_id":"restart-probe","confirmed":{"mention":"那个模块","canonical":"`+canon+`"}}`)
	if got["need_disambiguate"] == true {
		_ = cmd1.Process.Kill()
		t.Fatalf("[重启] 同进程带 confirmed 未复用: %v", got)
	}

	//  process -> heavystart(same  dataDir)
	if err := cmd1.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = cmd1.Process.Wait()

	addr2 := freePort(t)
	cmd2 := startRealServer(t, bin, addr2, dataDir)
	defer func() { _ = cmd2.Process.Kill() }()

	// **   confirmed** ⇒   clarification:   heavystartbefore  "confirm" hasbeserveservice  (also   )
	after := postProcess(t, "http://"+addr2, `{"text":"把那个模块改了","session_id":"restart-probe"}`)
	if after["need_disambiguate"] != true {
		t.Fatalf("[重启] 重启后不带 confirmed 却未回问 ⇒ 服务记住了会话态（行为证据证伪）: %v", after)
	}
}
