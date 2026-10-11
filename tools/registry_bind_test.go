package tools

import (
	"testing"

	"voicesign-harness/contract"
)

// A0: BindContract is the assembly-time path for customer adapter tools: it
// validates and binds in memory but must not require a contract dir or persist.
func TestRegistryBindContract(t *testing.T) {
	r, err := LoadContracts("") // built-ins only, no on-disk dir
	if err != nil {
		t.Fatalf("LoadContracts: %v", err)
	}
	c := contract.ToolContract{
		Name:    "crm_create_case",
		Version: "1.0",
		Caps:    []string{"create_case"},
		Params:  map[string]string{"customer": "string,required"},
		Risk:    map[string]string{"create_case": "medium"},
	}
	if err := r.BindContract(c); err != nil {
		t.Fatalf("BindContract: %v", err)
	}
	got, ok := r.Get("crm_create_case")
	if !ok {
		t.Fatal("bound contract not found via Get")
	}
	if got.Source != "customer" {
		t.Fatalf("Source=%q, want customer", got.Source)
	}
	if err := r.BindContract(c); err == nil {
		t.Fatal("expected error on rebind of the same contract")
	}
	if _, exists := r.Get("git"); !exists {
		t.Fatal("built-in git contract missing after bind")
	}
}

// An invalid customer contract must be rejected at bind time.
func TestRegistryBindContractRejectsInvalid(t *testing.T) {
	r, _ := LoadContracts("")
	invalid := contract.ToolContract{Name: "bad", Version: "1.0"} // no caps/params/risk
	if err := r.BindContract(invalid); err == nil {
		t.Fatal("expected validation error for incomplete contract")
	}
}
