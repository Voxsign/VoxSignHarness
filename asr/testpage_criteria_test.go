//go:build vhsui

// testpage_criteria_test.go —— 测试页判据（先红纪律）：能打开 + 一次输入 ⇒ 台账多一条。
package asr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// uiStore 是本 tag 自带的教词后端（避免依赖 vhs002 夹具里的实现）。
type uiStore struct {
	m      map[string]string
	taught map[string]bool
}

func (u *uiStore) Teach(term, canonical string) error {
	u.m[term] = canonical
	u.taught[term] = true
	return nil
}

func (u *uiStore) ClearTaught() int {
	n := 0
	for k := range u.taught {
		delete(u.m, k)
		delete(u.taught, k)
		n++
	}
	return n
}

func (u *uiStore) Rewrite(text string) (string, []Correction) {
	out := text
	hit := false
	for term, canon := range u.m {
		if strings.Contains(out, term) {
			out = strings.ReplaceAll(out, term, canon)
			hit = true
		}
	}
	if !hit {
		return text, nil
	}
	return out, []Correction{{Kind: "hotword", Confidence: 0.95, Evidence: "user_taught"}}
}

func newUIServer(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	pipe := NewPipeline(NewEngine(), nil, nil)
	store := &uiStore{m: map[string]string{}, taught: map[string]bool{}}
	pipe.Hot = store
	s := NewServer(pipe)
	s.DataDir = dir
	s.Teach = store.Teach
	s.ClearTaught = store.ClearTaught
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv.URL, filepath.Join(dir, "reallog.jsonl")
}

// 页面能打开（200 + 有输入框）。
func TestUIPageServesHTML(t *testing.T) {
	base, _ := newUIServer(t)
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("GET / → %d", resp.StatusCode)
	}
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "<textarea") {
		t.Errorf("页面缺少标准输入框（macOS 听写需要）")
	}
}

// **页面意义所在**：一次输入 ⇒ 真实台账多一条（含考察点所需字段）。
func TestUIOneInputAppendsRealLog(t *testing.T) {
	base, logPath := newUIServer(t)
	body := `{"text":"把哎欧劈艾斯接上"}`
	resp, err := http.Post(base+"/v1/testpage", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"raw", "corrected", "intent", "ask_back", "level", "ms"} {
		if _, ok := out[k]; !ok {
			t.Errorf("响应缺字段 %q —— 页面显示不全", k)
		}
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("真实台账未生成: %v", err)
	}
	lines := strings.Count(strings.TrimSpace(string(b)), "\n") + 1
	if lines != 1 {
		t.Errorf("一次输入应落 1 条，实际 %d", lines)
	}
	var rec RealLogRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Raw != "把哎欧劈艾斯接上" || rec.Intent == "" || rec.Level == "" {
		t.Errorf("台账字段不全: %+v", rec)
	}
}

// 看台账端点能算出三个考察点。
func TestUITestLogComputesMetrics(t *testing.T) {
	base, _ := newUIServer(t)
	for i := 0; i < 2; i++ {
		resp, err := http.Post(base+"/v1/testpage", "application/json", strings.NewReader(`{"text":"查一下库存"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	resp, err := http.Get(base + "/v1/testlog?n=10")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Total   int `json:"total"`
		Metrics struct {
			L0Share      float64 `json:"l0_share"`
			AskBackRate  float64 `json:"ask_back_rate"`
			DegradedRate float64 `json:"degraded_rate"`
		} `json:"metrics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 2 {
		t.Errorf("台账条数=%d，期望 2", out.Total)
	}
	if out.Metrics.L0Share != 1.0 {
		t.Errorf("L0 比例=%.2f（本地路径应全为 L0）", out.Metrics.L0Share)
	}
}
