package pipeline

// R13 (2026-10-09): unit coverage for the Observability (trace) and Governance
// (deny-once) P0 work — replayable audit trail + session-level rejection memory.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voicesign-harness/contract"
)

func TestTraceAppendAndFields(t *testing.T) {
	logDir := t.TempDir()
	rec := traceRec{
		RequestID:   "r13-test-1",
		TS:          "2026-10-09T12:00:00+03:00",
		ConvID:      "c1",
		Intent:      "EMAIL",
		Action:      "EMAIL",
		Receipts:    []traceReceipt{{Tool: "email", OK: true, StdoutLen: 42}},
		Attribution: "execution/ok",
		LoopMs:      1234,
		NetMs:       1200,
	}
	appendTrace(logDir, "c1", rec)
	p := filepath.Join(logDir, "context_slots", "trace-c1.jsonl")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("trace file not written: %v", err)
	}
	var got traceRec
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("trace line not valid JSON: %v", err)
	}
	if got.RequestID != "r13-test-1" || got.Action != "EMAIL" || got.LoopMs != 1234 {
		t.Fatalf("trace fields mismatch: %+v", got)
	}
	if len(got.Receipts) != 1 || got.Receipts[0].Tool != "email" || !got.Receipts[0].OK {
		t.Fatalf("receipts not captured: %+v", got.Receipts)
	}
	// append-only: a second turn adds a line, never truncates.
	appendTrace(logDir, "c1", rec)
	lines := strings.Split(strings.TrimSpace(string(mustRead(t, p))), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected append-only 2 lines, got %d", len(lines))
	}
}

func TestTraceSkipsEmptyRequestID(t *testing.T) {
	logDir := t.TempDir()
	appendTrace(logDir, "c1", traceRec{})
	if _, err := os.Stat(filepath.Join(logDir, "context_slots", "trace-c1.jsonl")); err == nil {
		t.Fatal("empty request_id must not create a trace file")
	}
}

func TestDenyLifecycle(t *testing.T) {
	logDir := t.TempDir()
	conv := "d1"
	if isDenied(logDir, conv, "install:codex") {
		t.Fatal("fresh session must not be denied")
	}
	recordDeny(logDir, conv, "install:codex")
	if !isDenied(logDir, conv, "install:codex") {
		t.Fatal("deny must be visible after record")
	}
	if isDenied(logDir, conv, "install:claude") {
		t.Fatal("different target must not be denied")
	}
	clearDeny(logDir, conv, "install:codex")
	if isDenied(logDir, conv, "install:codex") {
		t.Fatal("deny must clear after revive")
	}
}

func TestDenyKeyDerivation(t *testing.T) {
	cases := []struct {
		name string
		it   contract.Intent
		want string
	}{
		{"install", contract.Intent{Intent: contract.IntentInstall, Params: map[string]string{"packages": "codex"}}, "install:codex"},
		{"reply-targeted", contract.Intent{Intent: contract.IntentEmail, Params: map[string]string{"action": "reply", "target": "Sofia"}}, "email:reply:Sofia"},
		{"reply-bare", contract.Intent{Intent: contract.IntentEmail, Params: map[string]string{"action": "reply"}}, "email:reply:"},
		{"summary-no-key", contract.Intent{Intent: contract.IntentEmail, Params: map[string]string{"action": "summary"}}, ""},
	}
	for _, c := range cases {
		if got := denyKeyForIntent(c.it); got != c.want {
			t.Fatalf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestReviveTriggers(t *testing.T) {
	for _, s := range []string{"还是要装 codex", "算了做吧", "改成做邮件", "重新做", "还是要回那封邮件"} {
		if !isRevive(s) {
			t.Fatalf("expected revive for %q", s)
		}
	}
	for _, s := range []string{"装 codex", "先别管了", "处理邮件", "那封回复了没"} {
		if isRevive(s) {
			t.Fatalf("unexpected revive for %q", s)
		}
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return b
}
