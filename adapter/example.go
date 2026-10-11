package adapter

import (
	"fmt"
	"strings"

	"voicesign-harness/contract"
)

// ExampleAdapter is a copy-and-modify template showing how to onboard a new
// customer. It wires a single harmless tool (customer_echo) so the pattern is
// obvious. It is never applied unless the integrator explicitly constructs it
// and passes it to Apply.
//
// To onboard a real customer:
//  1. Copy this type into the customer's private integration package/repo.
//  2. Replace Meta with the customer's identity and configuration directory.
//  3. Replace Tools() with the customer's business tools, each as a
//     contract.ToolContract (caps/params/risk/spaces) plus a ToolHandler that
//     performs the real action and reads credentials only from env/config.
//  4. Record real, desensitized voice scenarios in AcceptanceCases() for the
//     A4 benchmark.
//  5. After buildOptions, call adapter.Apply(opts, yourAdapter{}) at startup.
type ExampleAdapter struct{}

// NewExampleAdapter returns the documented template adapter.
func NewExampleAdapter() ExampleAdapter { return ExampleAdapter{} }

// Meta returns the template customer's identity.
func (ExampleAdapter) Meta() Meta {
	return Meta{
		CustomerID:  "example-customer",
		BrandName:   "Example Co",
		DefaultLang: "en",
		Timezone:    "UTC",
	}
}

// Tools returns the template's single demonstration tool.
func (ExampleAdapter) Tools() []ToolBinding {
	return []ToolBinding{{
		Contract: contract.ToolContract{
			Name:          "customer_echo",
			Version:       "1.0",
			Caps:          []string{"echo"},
			Params:        map[string]string{"text": "string,required"},
			SideEffects:   []string{"none"},
			AllowedSpaces: []string{"global"},
			Risk:          map[string]string{"echo": "none"},
		},
		Handler: func(args map[string]any) (string, error) {
			t, _ := args["text"].(string)
			t = strings.TrimSpace(t)
			if t == "" {
				return "", fmt.Errorf("text is required")
			}
			return "echo: " + t, nil
		},
	}}
}

// AcceptanceCases returns a placeholder; replace with real recorded scenarios.
func (ExampleAdapter) AcceptanceCases() []AcceptanceCase {
	return []AcceptanceCase{{
		ID:         "ex-echo-1",
		UserSay:    "replace with a real customer voice utterance",
		ExpectTool: "customer_echo",
		ExpectOK:   true,
		Notes:      "Template only; record real, desensitized cases for the A4 benchmark.",
	}}
}
