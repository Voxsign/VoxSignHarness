// feedback_criteria_test.go —— 回馈（✔/✘）落盘 + 黑名单端点判据（§5.1 第 4/5 件；A8/A9/A10）。
//
// 默认门禁（无 tag）：本能力**已实现**，判据随默认门禁常跑（不依赖真实服务/网络）。
// 钉住：① ✔ ⇒ feedback.jsonl +1；② ✘ ⇒ +1 且**带原因**；③ 黑名单端点登记 + 钩子被调用；
// ④ 页面含 A2 要求的控制元素。
package asr

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newFeedbackServer 是自包含的测试服务（默认 tag 专用；vhsui 的 newUIServer 不可见）。
func newFeedbackServer(t *testing.T) (base string, feedbackPath string) {
	t.Helper()
	dir := t.TempDir()
	dict, err := NewDictionary(filepath.Join(dir, "custom-dictionary.json"))
	if err != nil {
		t.Fatal(err)
	}
	pipe := NewPipeline(NewEngine(), dict, nil)
	s := NewServer(pipe)
	s.DataDir = dir
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv.URL, filepath.Join(dir, "feedback.jsonl")
}

func postFeedback(t *testing.T, base, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(base+"/v1/feedback", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if code := resp.StatusCode; code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, out)
	}
	return out
}

func readFeedbackLines(t *testing.T, p string) []FeedbackRecord {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []FeedbackRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var rec FeedbackRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("feedback.jsonl 有非法行: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

// A8：点〔✔ 对〕⇒ feedback.jsonl 多一条。
func TestFeedbackAcceptedAppendsLine(t *testing.T) {
	base, p := newFeedbackServer(t)
	postFeedback(t, base, `{"text_raw":"哈牛斯","text_final":"harness","accepted":true,"source":"user_edit"}`)
	lines := readFeedbackLines(t, p)
	if len(lines) != 1 {
		t.Fatalf("[FB-01] ✔ 后 feedback.jsonl 应有 1 条，实际 %d", len(lines))
	}
	if !lines[0].Accepted {
		t.Errorf("[FB-01] accepted 应为 true：%+v", lines[0])
	}
	if lines[0].Raw != "哈牛斯" || lines[0].Corrected != "harness" {
		t.Errorf("[FB-01] 原文/纠错未记录：%+v", lines[0])
	}
	// 原有语义不破：accepted+有改动 ⇒ 词典仍登记（SCOPE-AUDIT-01 依赖）。
	resp, err := http.Post(base+"/v1/dictionary", "application/json", strings.NewReader(`{"op":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var dl map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&dl); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(dl)
	if !strings.Contains(string(b), "哈牛斯") {
		t.Errorf("[FB-01] 词典未登记 accepted 词条（原有语义被破坏）")
	}
}

// A9：点〔✘ 不对〕⇒ 多一条且**带原因**。
func TestFeedbackRejectedAppendsLineWithReason(t *testing.T) {
	base, p := newFeedbackServer(t)
	postFeedback(t, base, `{"text_raw":"把哎欧劈艾斯接上","text_final":"把aiops接上","accepted":false,"reason":"这个不该改"}`)
	lines := readFeedbackLines(t, p)
	if len(lines) != 1 {
		t.Fatalf("[FB-02] ✘ 后 feedback.jsonl 应有 1 条，实际 %d", len(lines))
	}
	if lines[0].Accepted {
		t.Errorf("[FB-02] accepted 应为 false：%+v", lines[0])
	}
	if lines[0].Reason != "这个不该改" {
		t.Errorf("[FB-02] 原因未记录：%+v", lines[0])
	}
}

// A9 边界：✘ 未填原因 ⇒ 记默认原因（字段恒非空，不许丢）。
func TestFeedbackRejectedDefaultsReason(t *testing.T) {
	base, p := newFeedbackServer(t)
	postFeedback(t, base, `{"text_raw":"x","text_final":"y","accepted":false}`)
	lines := readFeedbackLines(t, p)
	if len(lines) != 1 || lines[0].Reason != DefaultRejectReason {
		t.Fatalf("[FB-03] 默认原因应为 %q：%+v", DefaultRejectReason, lines)
	}
}

// A10 服务端：/v1/blacklist 登记并调用钩子；缺钩子 ⇒ 503（不假装支持）。
func TestBlacklistEndpointAddsAndNeedsHook(t *testing.T) {
	dir := t.TempDir()
	pipe := NewPipeline(NewEngine(), nil, nil)
	s := NewServer(pipe)
	s.DataDir = dir
	var gotTerm, gotNote string
	s.Blacklist = func(term, note string) error { gotTerm, gotNote = term, note; return nil }
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/blacklist", "application/json",
		strings.NewReader(`{"op":"add","term":"哎欧劈艾斯","note":"A10 验收"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true || gotTerm != "哎欧劈艾斯" || gotNote != "A10 验收" {
		t.Errorf("[BL-10] 端点未正确登记：%v hook=(%q,%q)", out, gotTerm, gotNote)
	}

	// 缺钩子 ⇒ 503。
	pipe2 := NewPipeline(NewEngine(), nil, nil)
	s2 := NewServer(pipe2)
	srv2 := httptest.NewServer(s2.Handler())
	defer srv2.Close()
	resp2, err := http.Post(srv2.URL+"/v1/blacklist", "application/json", strings.NewReader(`{"op":"add","term":"x","note":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("[BL-10] 缺钩子应 503，实际 %d", resp2.StatusCode)
	}
}

// A2 元素：页面含〔🎤 说话〕〔处理〕〔上传〕〔规划〕〔✔〕〔✘〕〔这个改错了〕。
func TestPageHasAcceptanceControls(t *testing.T) {
	base, _ := newFeedbackServer(t)
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 1<<16)
	n, _ := resp.Body.Read(buf)
	page := string(buf[:n])
	for _, want := range []string{"🎤 说话", "处理", "规划", "✔ 对", "✘ 不对", "这个改错了"} {
		if !strings.Contains(page, want) {
			t.Errorf("[A2] 页面缺 %q", want)
		}
	}
	if !strings.Contains(page, `type="file"`) {
		t.Errorf("[A2] 页面缺上传（文件输入）")
	}
}
