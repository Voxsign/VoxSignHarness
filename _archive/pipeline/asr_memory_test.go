package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"voicesign-harness/contract"
)

// ASR 沉淀进入记忆槽（2026-10-04）：feedback/blacklist/dictionary 教词 → "我学到的"摘要。
func TestLoadASRMemory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "feedback.jsonl"), []byte(
		`{"at":"2026-10-04T00:00:00Z","raw":"彼得周点com","corrected":"model.peterzou.com","accepted":true,"reason":"","source":"user"}`+"\n"+
			`{"at":"2026-10-04T00:01:00Z","raw":"曼苏","corrected":"Mansour","accepted":false,"reason":"user_marked_wrong","source":"user"}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "blacklist.json"), []byte(`{"曼苏":"改错了，应为 Mansour"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "dictionary.json"), []byte(`{"version":1,"terms":[{"term":"Mansour","variants":["曼苏"],"category":"人名","source":"manual"}]}`), 0o644)

	o := &Options{ASRDataDir: dir}
	mem := o.loadASRMemory()
	for _, want := range []string{"彼得周点com", "model.peterzou.com", "曼苏", "Mansour", "黑名单", "教词"} {
		if !containsStr(mem, want) {
			t.Fatalf("摘要缺 %q：got %q", want, mem)
		}
	}
	// 空目录 → 不注入
	if (&Options{ASRDataDir: t.TempDir()}).loadASRMemory() != "" {
		t.Fatal("空目录应返回空")
	}
	// ASRDataDir 空 → 不注入
	if (&Options{}).loadASRMemory() != "" {
		t.Fatal("ASRDataDir 空应返回空")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// memoryContext 从 intent.Context 提取记忆拼进 LLM prompt（2026-10-04 修复纸面记忆）。
func TestMemoryContext(t *testing.T) {
	o := &Options{}
	it := contract.Intent{Context: []string{
		"asr-memory: ✔确认:彼得周点com→model.peterzou.com；✘标错:曼苏→Mansour",
		"project-map:voicesign",
	}}
	m := o.memoryContext(it)
	for _, want := range []string{"ASR 沉淀", "model.peterzou.com", "Mansour", "项目背景", "voicesign"} {
		if !containsStr(m, want) {
			t.Fatalf("memoryContext 缺 %q：got %q", want, m)
		}
	}
	// 空 Context → 空（不污染 prompt）
	if o.memoryContext(contract.Intent{}) != "" {
		t.Fatal("空 Context 应返回空")
	}
	// memoryBlock：空 → ""；非空 → 带换行前缀
	if memoryBlock("") != "" {
		t.Fatal("memoryBlock 空应返回空")
	}
	if !containsStr(memoryBlock("x"), "\n\n") {
		t.Fatal("memoryBlock 非空应带换行前缀")
	}
}
