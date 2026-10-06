// dict_test.go --   1 approve   **default forbid**  (   tag,   `go test ./...`   ). 
//
// and `//go:build vhs002`   data same:    connect   (Dictionary/Tracer/Pipeline), 
//  dependency HTTP serveservice, because default forbidthen     new  . 
package asr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestDict(t *testing.T) (*Dictionary, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "custom-dictionary.json")
	d, err := NewDictionary(path)
	if err != nil {
		t.Fatalf("NewDictionary: %v", err)
	}
	return d, path
}

// TestDictionaryPersistAcrossInstances:   afternew example finish   (heavystart  ). 
func TestDictionaryPersistAcrossInstances(t *testing.T) {
	d, path := newTestDict(t)
	if err := d.Add(DictionaryEntry{RawSpeech: "哈牛斯", Target: "harness", Priority: 7, Source: "user_edit"}); err != nil {
		t.Fatal(err)
	}
	re, err := NewDictionary(path)
	if err != nil {
		t.Fatal(err)
	}
	list := re.List()
	if len(list) != 1 || list[0].RawSpeech != "哈牛斯" || list[0].Target != "harness" {
		t.Fatalf("重启后条目不符: %+v", list)
	}
	if list[0].CreatedAt == "" || list[0].Source != "user_edit" {
		t.Fatalf("审计字段未持久化: %+v", list[0])
	}
	if text, _ := re.Apply("哈牛斯接上"); text != "harness接上" {
		t.Fatalf("重启后纠错失效: %q", text)
	}
}

// TestDictionaryVoiceInstructionAddsHomophone: langaudiorefer  ->   write  ->  audio back. 
func TestDictionaryVoiceInstructionAddsHomophone(t *testing.T) {
	e, ok := ParseVoiceAdd("记住，冀总是冀中的冀")
	if !ok || e.Target != "冀总" {
		t.Fatalf("语音指令解析失败: ok=%v entry=%+v", ok, e)
	}
	d, _ := newTestDict(t)
	if err := d.Add(e); err != nil {
		t.Fatal(err)
	}
	got, corrs := d.Apply("季总看一下")
	if got != "冀总看一下" {
		t.Fatalf("近音召回失败: %q → %q", "季总看一下", got)
	}
	if len(corrs) == 0 || corrs[0].Kind != "dictionary" {
		t.Fatalf("缺可观测留痕: %+v", corrs)
	}
}

// TestDictionaryDeleteNeedsConfirm: noconfirm delete,    ( line #3). 
func TestDictionaryDeleteNeedsConfirm(t *testing.T) {
	d, path := newTestDict(t)
	if err := d.Add(DictionaryEntry{Target: "冀总", Source: "voice"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := d.Delete("冀总", false)
	if err != nil || deleted {
		t.Fatalf("未确认却删除: deleted=%v err=%v", deleted, err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("未确认的删除改动了文件")
	}
	if deleted, err := d.Delete("冀总", true); err != nil || !deleted {
		t.Fatalf("确认后未删除: deleted=%v err=%v", deleted, err)
	}
	if len(d.List()) != 0 {
		t.Fatalf("确认后条目仍在: %+v", d.List())
	}
}

// TestDictionaryHotReload: out modify JSON fileafter Reload  i.e.occur . 
func TestDictionaryHotReload(t *testing.T) {
	d, path := newTestDict(t)
	if changed, _ := d.Reload(); changed {
		t.Fatal("未改动却报告热加载")
	}
	content := `{"version":3,"entries":[{"raw_speech":"哈牛斯","target":"harness","scope":"global","priority":9,"source":"manual"}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := d.Reload()
	if err != nil || !changed {
		t.Fatalf("热加载未生效: changed=%v err=%v", changed, err)
	}
	if text, _ := d.Apply("哈牛斯接上"); text != "harness接上" {
		t.Fatalf("热加载后纠错失效: %q", text)
	}
}

// TestDictionaryBadJSONFailsOpen:   JSON  overwrite fast ,   panic(needrequire 4.9). 
func TestDictionaryBadJSONFailsOpen(t *testing.T) {
	d, path := newTestDict(t)
	if err := d.Add(DictionaryEntry{RawSpeech: "哈牛斯", Target: "harness"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Reload(); err == nil {
		t.Fatal("坏 JSON 未报错")
	}
	if text, _ := d.Apply("哈牛斯接上"); text != "harness接上" {
		t.Fatalf("坏 JSON 后旧快照丢失: %q", text)
	}
}

// TestTracerAppendOnlyJSONL: traceonly  ,    resolve , charseg safety. 
func TestTracerAppendOnlyJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traces-asr.jsonl")
	tr, err := NewTracer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()
	for _, step := range []string{"retain", "clean"} {
		if err := tr.Append(TraceRecord{RequestID: "r1", Step: step, Ms: 0.1, Source: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("期望 2 行轨迹，实际 %d", len(lines))
	}
	for i, line := range lines {
		var rec TraceRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("第 %d 行非法: %v", i, err)
		}
		if rec.Step == "" || rec.At == "" || rec.Source == "" {
			t.Fatalf("第 %d 行字段缺失: %+v", i, rec)
		}
	}
}

// TestPipelineStepsAndTrace: manageline    ,   all  . 
func TestPipelineStepsAndTrace(t *testing.T) {
	d, _ := newTestDict(t)
	tracePath := filepath.Join(t.TempDir(), "traces-asr.jsonl")
	tr, err := NewTracer(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()

	pipe := NewPipeline(NewEngine(), d, tr)
	res := pipe.Process("嗯那个呃记一下这个想法", "s1")
	if res.Text != "记一下这个想法" {
		t.Fatalf("清洗结果: %q", res.Text)
	}
	want := []string{"retain", "clean", "dict", "punctuate"}
	if len(res.Steps) != len(want) {
		t.Fatalf("步数 %d，期望 %d: %+v", len(res.Steps), len(want), res.Steps)
	}
	for i, w := range want {
		if res.Steps[i].Step != w {
			t.Errorf("第 %d 步 = %q，期望 %q", i, res.Steps[i].Step, w)
		}
	}
	data, _ := os.ReadFile(tracePath)
	if n := len(strings.Split(strings.TrimSpace(string(data)), "\n")); n != len(want) {
		t.Errorf("轨迹行数 %d，期望 %d", n, len(want))
	}
}

// TestPipelineWithoutDictKeepsEngineSemantics: word asempty/null time, manageline  modifychange  close . 
func TestPipelineWithoutDictKeepsEngineSemantics(t *testing.T) {
	eng := NewEngine()
	pipe := NewPipeline(eng, nil, nil)
	for _, raw := range []string{"嗯那个呃记一下这个想法", "就是这里时候要不要用deept的", "那个那个那个"} {
		want := eng.Correct(CorrectRequest{Raw: raw})
		got := pipe.Process(raw, "")
		if got.Text != want.Text || got.Punctuated != want.Punctuated {
			t.Errorf("[%q] 管线改变了引擎语义: Text %q→%q, Punctuated %q→%q",
				raw, want.Text, got.Text, want.Punctuated, got.Punctuated)
		}
	}
}
