package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"voicesign-harness/config"
)

// 记忆模拟装 · 槽回读单测（2026-10-03）：
// 验证 指代固化上下文槽 的（1）最近优先（2）document 全文恢复（3）会话隔离——C/R 双证。
func TestMemorySlotAnaphora(t *testing.T) {
	dir := t.TempDir()
	// 造两条槽：会话 mem-A1 先定义《需求A》再定义《需求B》
	slotPath := filepath.Join(dir, "context_slots", "mem-A1.jsonl")
	os.MkdirAll(filepath.Dir(slotPath), 0o755)
	lines := []string{
		`{"ts":"2026-10-03T10:00:00Z","conv_id":"mem-A1","text":"请实现《需求A》这份","doc_head":"A head","doc_full":"document-A-full-AAAA","has_doc":true}`,
		`{"ts":"2026-10-03T10:01:00Z","conv_id":"mem-A1","text":"请实现《需求B》那份","doc_head":"B head","doc_full":"document-B-full-BBBB","has_doc":true}`,
	}
	os.WriteFile(slotPath, []byte(lines[0]+"\n"+lines[1]+"\n"), 0o644)

	o := &Options{Cfg: &config.Config{}}
	o.Cfg.Global.LogDir = dir
	o.ConvID = "mem-A1"

	// (1) 最近优先：倒序扫槽，应命中《需求B》
	if td := o.referTargetFromSlots(); td != "需求B" {
		t.Fatalf("最近优先失败：got %q want 需求B", td)
	}
	// (2) document 全文恢复：最近 has_doc 记录 → doc_full
	if d := slotLatestDocument(o.logDir(), o.ConvID); d != "document-B-full-BBBB" {
		t.Fatalf("document 恢复失败：got %q", d)
	}
	// (3) 会话隔离：mem-X 无槽 → 不猜（空）
	if td := (&Options{Cfg: &config.Config{}, ConvID: "mem-X"}).referTargetFromSlots(); td != "" {
		t.Fatalf("会话隔离失败：跨会话猜出 %q", td)
	}
	// (4) 无 document 的会话：恢复空（不编造）
	os.WriteFile(slotPath, []byte(`{"ts":"2026-10-03T10:02:00Z","conv_id":"mem-A1","text":"那个事","doc_head":"","doc_full":"","has_doc":false}`+"\n"), 0o644)
	if d := slotLatestDocument(o.logDir(), "mem-A1"); d != "" {
		t.Fatalf("无 document 应恢复空，got %q", d)
	}
}
