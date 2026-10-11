package tools

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"voicesign-harness/contract"
)

// A0: a customer adapter registers a real business handler and it executes
// through the standard Receipt path used by built-in tools.
func TestExecutorCustomerHandlerExec(t *testing.T) {
	e := &Executor{BaseDir: t.TempDir()}
	called := false
	err := e.RegisterHandler("crm_create_case", func(args map[string]any) (string, error) {
		called = true
		customer, _ := args["customer"].(string)
		return "case-created:" + customer, nil
	})
	if err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	if e.HandlerCount() != 1 {
		t.Fatalf("HandlerCount=%d, want 1", e.HandlerCount())
	}
	recv, err := e.Exec("crm_create_case", map[string]any{"customer": "ACME"}, contract.ToolContract{Name: "crm_create_case"})
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if !called {
		t.Fatal("customer handler was not invoked")
	}
	if !recv.OK || !strings.Contains(recv.Stdout, "case-created:ACME") {
		t.Fatalf("unexpected receipt: %+v", recv)
	}
}

// Registration must fail closed: no empty names, nil handlers, duplicates, or
// shadowing of built-in tools.
func TestExecutorRegisterHandlerRejects(t *testing.T) {
	e := &Executor{}
	noop := func(map[string]any) (string, error) { return "", nil }

	if err := e.RegisterHandler("git", noop); err == nil {
		t.Fatal("expected error when shadowing built-in tool git")
	}
	if err := e.RegisterHandler("  ", noop); err == nil {
		t.Fatal("expected error for empty tool name")
	}
	if err := e.RegisterHandler("nil_tool", nil); err == nil {
		t.Fatal("expected error for nil handler")
	}
	if err := e.RegisterHandler("ok_tool", noop); err != nil {
		t.Fatalf("first registration should succeed, got %v", err)
	}
	if err := e.RegisterHandler("ok_tool", noop); err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

// A handler returning an error surfaces as a non-OK receipt with Err populated,
// never as a false success.
func TestExecutorCustomerHandlerError(t *testing.T) {
	e := &Executor{}
	if err := e.RegisterHandler("boom", func(map[string]any) (string, error) {
		return "", fmt.Errorf("kaboom")
	}); err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	recv, err := e.Exec("boom", nil, contract.ToolContract{Name: "boom"})
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if recv.OK || !strings.Contains(recv.Err, "kaboom") {
		t.Fatalf("want non-OK receipt with Err kaboom, got %+v", recv)
	}
}

// A handler that overruns the executor timeout produces a timeout receipt
// instead of hanging the run.
func TestExecutorCustomerHandlerTimeout(t *testing.T) {
	e := &Executor{Timeout: 20 * time.Millisecond}
	if err := e.RegisterHandler("slow", func(map[string]any) (string, error) {
		time.Sleep(300 * time.Millisecond)
		return "late", nil
	}); err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	recv, err := e.Exec("slow", nil, contract.ToolContract{Name: "slow"})
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if recv.OK || !strings.Contains(recv.Err, "timed out") {
		t.Fatalf("want non-OK timeout receipt, got %+v", recv)
	}
}

// Tools without a handler and outside the built-in switch still report
// Blocked (the pre-A0 behaviour is preserved).
func TestExecutorUnknownToolStillBlocked(t *testing.T) {
	e := &Executor{}
	recv, err := e.Exec("not_a_real_tool", nil, contract.ToolContract{Name: "not_a_real_tool"})
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if recv.OK || !strings.Contains(recv.Blocked, "not implemented") {
		t.Fatalf("want Blocked receipt for unknown tool, got %+v", recv)
	}
}
