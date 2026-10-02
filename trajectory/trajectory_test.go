package trajectory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"voicesign-harness/contract"
)

func todayFile(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(dir, "trajectory-"+time.Now().Format("20060102")+".jsonl")
}

func TestOpenCreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	tr, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer tr.Close()
	fi, err := os.Stat(todayFile(t, dir))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm want 0600, got %v", fi.Mode().Perm())
	}
}

func TestWriteAppendsParseableLines(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries := []Entry{
		{RequestID: "r1", Kind: KindInputRaw, Content: "帮我打开美墅的文件夹看看有什么"},
		{RequestID: "r1", Turn: 1, Kind: KindIntent, Intent: &contract.Intent{Intent: contract.IntentFileList, Confidence: 0.8}},
		{RequestID: "r1", Kind: KindReceipts, Receipts: []contract.Receipt{{Seq: 1, Tool: "list_dir", OK: true, Stdout: "x"}}},
	}
	for _, e := range entries {
		if err := tr.Write(e); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(todayFile(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}
	for i, ln := range lines {
		var out Entry
		if err := json.Unmarshal([]byte(ln), &out); err != nil {
			t.Fatalf("line %d not parseable: %v", i, err)
		}
	}
	var first Entry
	json.Unmarshal([]byte(lines[0]), &first)
	if first.Content != "帮我打开美墅的文件夹看看有什么" {
		t.Fatalf("raw content mismatch: %q", first.Content)
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	const n = 20
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Write(Entry{RequestID: "r", Kind: KindStart, Content: "x"})
		}()
	}
	wg.Wait()
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(todayFile(t, dir))
	if got := strings.Count(string(data), "\n"); got != n {
		t.Fatalf("want %d lines, got %d", n, got)
	}
}
