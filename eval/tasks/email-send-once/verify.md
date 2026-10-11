# Eval task: email-send-once

A reliability task for the kernel's outbound-mail path. It asserts that one
logical email action produces exactly one external side effect under
concurrency, and that absence of a provider receipt can never be reported as
success. The automated validators live in `mail/guaranteed_test.go`,
`mail/tracker_test.go` and `action/action_test.go`; run them with
`go test -race ./action/ ./mail/`.

## Verification standard

```yaml
owner: harness-maintainer
verify:
  claim: "Under concurrent scheduler ticks for one tenant and Message-ID, the transport sender is invoked exactly once, and a missing provider receipt leaves the action in UNKNOWN rather than SUCCEEDED."
  method: test
  evidence:
    level: E1
    kind: automated-go-test
    ref: mail/guaranteed_test.go
  threshold: "exactly 1 sender invocation across 20 concurrent replicas and 0 duplicate side effects"
  counterexample: "After a single tick, two .eml files sharing one Message-ID are present in the sent directory, or the ledger records SUCCEEDED while the sender returned an empty provider identifier."
  verdict_states: [met, unverified, not_met]
  approver: independent-reviewer
  falsifier: "Any reviewer who runs the race-enabled mail test package and observes a second sender invocation for the same Message-ID, or a SUCCEEDED ledger record produced without a provider receipt."
```
