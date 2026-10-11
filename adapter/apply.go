package adapter

import (
	"fmt"
	"strings"

	"voicesign-harness/pipeline"
)

// Apply wires a customer adapter into an already-assembled pipeline.Options
// (the value returned by buildOptions). It binds each business tool's contract
// into the tool registry and its handler into the executor; every space, risk
// and confirmation gate continues to be enforced by the kernel. Call it once
// at startup, before any pipeline.Run.
//
// Apply is intentionally fail-closed: a malformed adapter prevents startup
// rather than silently shipping a deployment that cannot run its tools.
func Apply(o *pipeline.Options, a CustomerAdapter) error {
	if o == nil {
		return fmt.Errorf("adapter.Apply: nil pipeline.Options (call buildOptions first)")
	}
	if a == nil {
		return fmt.Errorf("adapter.Apply: nil adapter")
	}
	meta := a.Meta()
	if strings.TrimSpace(meta.CustomerID) == "" {
		return fmt.Errorf("adapter.Apply: meta.customer_id is required")
	}
	if o.Tools == nil {
		return fmt.Errorf("adapter.Apply(%s): Options.Tools not initialized", meta.CustomerID)
	}
	if o.Exec == nil {
		return fmt.Errorf("adapter.Apply(%s): Options.Exec not initialized", meta.CustomerID)
	}

	seen := map[string]bool{}
	for i, b := range a.Tools() {
		name := strings.TrimSpace(b.Contract.Name)
		if name == "" {
			return fmt.Errorf("adapter.Apply(%s): tool #%d has an empty contract name", meta.CustomerID, i+1)
		}
		if seen[name] {
			return fmt.Errorf("adapter.Apply(%s): duplicate tool %q", meta.CustomerID, name)
		}
		seen[name] = true
		if b.Handler == nil {
			return fmt.Errorf("adapter.Apply(%s): tool %q has a nil handler", meta.CustomerID, name)
		}

		c := b.Contract
		c.Name = name
		if err := o.Tools.BindContract(c); err != nil {
			return fmt.Errorf("adapter.Apply(%s): tool %q contract: %w", meta.CustomerID, name, err)
		}
		if err := o.Exec.RegisterHandler(name, b.Handler); err != nil {
			return fmt.Errorf("adapter.Apply(%s): tool %q handler: %w", meta.CustomerID, name, err)
		}
	}
	return nil
}
