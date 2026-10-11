// Package adapter defines the standard contract between the generic VoxSign
// harness kernel and a thin, per-customer integration layer.
//
// The kernel (the pipeline: intent -> plan -> space/risk/confirm gates ->
// orchestration -> verification -> receipt) stays customer-agnostic. A customer
// deployment supplies a CustomerAdapter that declares who the customer is, the
// real business tools the kernel is allowed to call, and the realistic
// acceptance cases used to measure the deployment. Onboarding a new customer
// means writing one adapter — the business actions as ToolHandlers — without
// changing the kernel, and without forking its executor.
package adapter

import (
	"voicesign-harness/contract"
	"voicesign-harness/tools"
)

// Meta identifies a customer deployment and its presentation defaults.
type Meta struct {
	// CustomerID is a stable machine identifier, e.g. "acme-demo".
	CustomerID string `json:"customer_id"`
	// BrandName is shown to users and on receipts.
	BrandName string `json:"brand_name"`
	// DefaultLang is "en" or "zh"; empty lets the kernel auto-detect from input.
	DefaultLang string `json:"default_lang"`
	// Timezone is an IANA name, e.g. "Asia/Riyadh".
	Timezone string `json:"timezone,omitempty"`
	// ConfigDir optionally holds data-driven customer configuration consumed by
	// the existing assembly: model-center.json, spaces/*.json, contracts/*.json
	// and constitution.json. Secrets are referenced by env-var name only; the
	// adapter never embeds keys or internal endpoints.
	ConfigDir string `json:"config_dir,omitempty"`
}

// ToolBinding pairs a tool's declared contract (the caps/params/risk/spaces the
// kernel's gates reason over) with its real execution handler (the customer
// action that does the work).
type ToolBinding struct {
	Contract contract.ToolContract
	// Handler is a leaf business action. Orchestration, confirmation, retries,
	// verification and tracing stay in the kernel; the handler only performs
	// the customer-specific work (e.g. create a CRM case, fetch an attachment).
	Handler tools.ToolHandler
}

// AcceptanceCase is one realistic, replayable scenario for the deployment's
// benchmark (A4). It is the quantitative basis for the understanding/action
// rate target. Record cases from real customer voice sessions and strip
// secrets and PII so they can be replayed offline.
type AcceptanceCase struct {
	ID         string `json:"id"`
	UserSay    string `json:"user_say"`                // the actual spoken utterance
	ExpectTool string `json:"expect_tool,omitempty"`  // business tool expected to fire
	ExpectOK   bool   `json:"expect_ok"`              // expected end-to-end success
	Notes      string `json:"notes,omitempty"`
}

// CustomerAdapter is the thin per-customer integration contract. Implement it
// in a separate package (and normally a separate private repository) per
// customer; the public kernel never imports a concrete customer adapter.
type CustomerAdapter interface {
	Meta() Meta
	Tools() []ToolBinding
	// AcceptanceCases may return nil until the deployment has recorded cases.
	AcceptanceCases() []AcceptanceCase
}
