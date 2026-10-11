# SPEC v0.8 — Tool Interface (T)

The tool interface is the narrow, validated seam between the kernel and any
capability it can call. Built-in tools and customer adapters register
through the same contract; the kernel never reaches around it.

## Contract

- A tool is described by a declarative contract (name, parameters, risk
  class). Registration validates the contract and rejects missing or
  malformed fields, even when the caller is pre-approved.
- Unknown or unapproved tools stay blocked; there is no implicit exec path.
- A customer handler is just another contract binding. It receives the
  request context (so cancellation and deadlines propagate), and its
  errors and timeouts are surfaced as failures rather than swallowed.
- The adapter contract carries customer metadata and acceptance cases; the
  shipped example adapter must both apply and execute and expose at least
  one acceptance case.
- Toggling a tool's approval changes only that tool; it never opens a
  blanket path for sibling tools.

## Pinned validators

- `TestExecutorRegisterHandlerRejects`
- `TestExecutorUnknownToolStillBlocked`
- `TestExecutorCustomerHandlerTimeout`
- `TestExecutorCustomerHandlerError`
- `TestRegister_InvalidRejectedEvenApproved`
- `TestRegister_ToggleApproved`
- `TestRegistryBindContract`
- `TestRegistryBindContractRejectsInvalid`
- `TestExampleAdapterApplyAndExec`
- `TestExampleAdapterHasAcceptanceCase`
