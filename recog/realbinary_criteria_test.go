//go:build vhsreal

// realbinary_criteria_test.go -- K9 **   restrict**endtoend(   "  "): 
//
//	①   andstart  process(  httptest), POST /v1/correct   out  because wordbutmodifychange
//	② revexample: L2 nodiffname ->  outandbaseline  
//	③ start day     to L2  new(diffname num/  /status)
//
// L2 use**base   /api/services**(deterministic,  dependencyout );  endpoint hashuman    . 
package recog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeServices(t *testing.T, aliases map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/services" {
			http.NotFound(w, r)
			return
		}
		type svc struct {
			Name    string   `json:"name"`
			Aliases []string `json:"aliases"`
		}
		var list []svc
		for alias, name := range aliases {
			list = append(list, svc{Name: name, Aliases: []string{alias}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ts": "2026-10-03T00:00:00Z", "services": list})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// startBinary      restrictandbygive  L2 endpointstart ; returnback baseURL andday   . 
func startBinary(t *testing.T, servicesURL string) (string, *bytes.Buffer) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vhs-asr")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vhs-asr")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("编译真二进制失败: %v\n%s", err, out)
	}
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	logs := &bytes.Buffer{}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"VHS_ASR_ADDR="+addr,
		"VHS_ASR_DATA="+t.TempDir(),
		"VHS_SERVICES_URL="+servicesURL,
	)
	cmd.Stderr = logs
	cmd.Stdout = logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动真进程失败: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })

	base := "http://" + addr
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return base, logs
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("真进程未在 10s 内就绪；日志:\n%s", logs.String())
	return "", logs
}

func postText(t *testing.T, base, text string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	resp, err := http.Post(base+"/v1/correct", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Text
}

// ①    restrict:  wordmodifychange out + ③ start day  see L2  new
func TestK9RealBinaryHotwordChangesOutput(t *testing.T) {
	l2 := fakeServices(t, map[string]string{"爱ops": "aiops-portal"})
	base, logs := startBinary(t, l2.URL+"/api/services")
	if got := postText(t, base, "把爱ops接上"); got != "把aiops-portal接上" {
		t.Fatalf("[K9-真二进制①] 输出未因热词改变: %q", got)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(logs.String(), "L2 缓存：") {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(logs.String(), "L2 缓存：") {
		t.Errorf("[K9-真二进制③] 启动日志没有 L2 刷新记录:\n%s", logs.String())
	}
	t.Logf("启动日志: %s", strings.TrimSpace(logs.String()))
}

// ② revexample: L2 nodiffname ->  outandbaseline  
func TestK9RealBinaryEmptyRegistryUnchanged(t *testing.T) {
	l2 := fakeServices(t, map[string]string{})
	base, _ := startBinary(t, l2.URL+"/api/services")
	if got := postText(t, base, "把爱ops接上"); got != "把爱ops接上" {
		t.Fatalf("[K9-真二进制②] 空注册表却改变了输出: %q", got)
	}
}
