//go:build vhs002

// vhs002_persist_test.go —— 第 1 批新增的判据（不改动已有判据，只补强）。
//
// 已有 SCOPE-DICT-01 只要求 VHS_ASR_BIN 非空、并不真的验证"重启不丢"；
// 这里补一条**真的**重启等价判据，以及一条轨迹可复现判据。
package asr

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// SCOPE-DICT-02：词典落盘后，用同一个文件新建运行时可完整恢复（等价重启）。
func TestSCOPEDICT02RestartPersists(t *testing.T) {
	base := serviceBase(t)
	dictPath := os.Getenv("VHS_ASR_DICT")
	if dictPath == "" {
		t.Fatalf("[criterion] 需要 VHS_ASR_DICT（未配置 → 先红）")
	}
	postJSON(t, base+"/v1/dictionary", `{"op":"add","raw_speech":"哈牛斯","target":"harness","priority":7,"source":"user_edit"}`)

	// 模拟进程重启：同一文件重新加载。
	restarted, err := NewDictionary(dictPath)
	if err != nil {
		t.Fatalf("[SCOPE-DICT-02] 重启加载失败: %v", err)
	}
	found := false
	for _, e := range restarted.List() {
		if e.RawSpeech == "哈牛斯" && e.Target == "harness" {
			found = true
			if e.Source != "user_edit" {
				t.Errorf("[SCOPE-DICT-02] source 未持久化: %q", e.Source)
			}
			if e.CreatedAt == "" {
				t.Errorf("[SCOPE-DICT-02] created_at 未持久化")
			}
		}
	}
	if !found {
		t.Fatalf("[SCOPE-DICT-02] 重启后词条丢失：%+v", restarted.List())
	}
	text, _ := restarted.Apply("哈牛斯接上")
	if text != "harness接上" {
		t.Errorf("[SCOPE-DICT-02] 重启后纠错失效：%q", text)
	}
}

// SCOPE-TRACE-02：同一输入重放，轨迹的**步骤与来源序列**必须一致（可复现）。
func TestSCOPETRACE02Replayable(t *testing.T) {
	base := serviceBase(t)
	tracePath := os.Getenv("VHS_ASR_TRACES")
	if tracePath == "" {
		t.Fatalf("[criterion] 需要 VHS_ASR_TRACES（未配置 → 先红）")
	}
	const text = "记一下这个想法"
	postJSON(t, base+"/v1/process", `{"text":"`+text+`","session_id":"replay"}`)
	postJSON(t, base+"/v1/process", `{"text":"`+text+`","session_id":"replay"}`)

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("[SCOPE-TRACE-02] 读轨迹失败: %v", err)
	}
	// 按 request_id 分组，取最后两次请求。
	var order []string
	groups := map[string][]TraceRecord{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec TraceRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("[SCOPE-TRACE-02] 轨迹行非法: %q", line)
		}
		if _, ok := groups[rec.RequestID]; !ok {
			order = append(order, rec.RequestID)
		}
		groups[rec.RequestID] = append(groups[rec.RequestID], rec)
	}
	if len(order) < 2 {
		t.Fatalf("[SCOPE-TRACE-02] 轨迹里请求不足 2 次：%d", len(order))
	}
	a := signature(groups[order[len(order)-2]])
	b := signature(groups[order[len(order)-1]])
	if a != b {
		t.Errorf("[SCOPE-TRACE-02] 重放步骤序列不一致：\n%q\n%q", a, b)
	}
}

func signature(recs []TraceRecord) string {
	parts := make([]string, 0, len(recs))
	for _, r := range recs {
		parts = append(parts, r.Step+"@"+r.Source)
	}
	return strings.Join(parts, ",")
}
