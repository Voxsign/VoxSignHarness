# Kernel ↔ Customer Adapter Contract

Status: **A0 landed** · Audience: integrators onboarding a new customer onto the VoxSign harness kernel.

## 1. Why this contract exists

The kernel is generic and open source: it turns a spoken request into intent, a
plan, gated actions, verification and a receipt. Customer work — reading a
mailbox, downloading an attachment, creating a CRM case, filing into a document
store — is private, different for every customer, and must not be forked into
the kernel.

Before A0 the executor only knew the built-in development tools
(`git/file/search/test/run/verify/remote-desktop/weather`). A business request
could be understood but had no executable action, so the harness could *hear* a
task yet not *do* it. A0 adds one narrow seam: real customer actions plug in as
handlers **behind the kernel's existing safety gates**, without modifying the
executor's switch and without forking the kernel.

The goal is that onboarding a new customer means writing one thin adapter, not
maintaining a fork.

## 2. Shape

```
voice / iOS app
      │  utterance
      ▼
┌────────────────────── generic kernel (open source, customer-agnostic) ──────────────┐
│ ASR → intent → plan → space check → risk/confirm gate → orchestrate → verify → log  │
│        ▲ modelcenter (local + external channels)       ▲ spaces · constitution      │
└────────┼──────────────────────────────────────────────┼─────────────────────────────┘
         │ ToolBinding.Contract (caps/params/risk)       │ data-driven config (ConfigDir)
         ▼ ToolBinding.Handler (one leaf business action)
┌────────────────── thin customer adapter (private, one per customer) ────────────────┐
│ email fetch/send · attachments · CRM cases · document store · customer watchdog …   │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

## 3. The contract

A customer deployment implements one interface:

```go
type CustomerAdapter interface {
    Meta() Meta
    Tools() []ToolBinding
    AcceptanceCases() []AcceptanceCase // may be nil until cases are recorded
}
```

### Meta

Who the deployment is and how it presents itself: `CustomerID`, `BrandName`,
`DefaultLang` (`en`/`zh`), `Timezone`, and an optional `ConfigDir` holding
data-driven configuration (`model-center.json`, `spaces/*.json`,
`contracts/*.json`, `constitution.json`). Secrets are referenced by environment
variable name only — never embedded.

### ToolBinding = contract + handler

```go
type ToolBinding struct {
    Contract contract.ToolContract // what the tool is and how risky each cap is
    Handler  tools.ToolHandler     // the leaf action that does the real work
}
```

The **contract** is what the kernel's gates reason over:

| Field | Purpose |
|---|---|
| `Caps` | The action verbs the tool exposes. |
| `Params` | Declared parameters (validated before execution). |
| `Risk` | Risk level per cap: `none/low/medium/high/irreversible`. `high`/`irreversible` drive the kernel's confirmation gate. |
| `AllowedSpaces` | Spaces in which the tool may run (boundary enforcement). |
| `SideEffects` | Declared effects for audit and confirmation text. |

The **handler** performs the customer-specific work and returns a result string
or an error. It is a *leaf*: it does not plan, retry, confirm or verify — the
kernel does all of that. A handler error becomes a non-OK receipt; it can never
produce a false success.

### AcceptanceCase

One realistic, replayable scenario (`UserSay`, expected tool, expected end-to-end
OK) recorded from a real voice session and stripped of secrets/PII. These are
the quantitative benchmark for the deployment (see §7).

## 4. Safety model — what always stays in the kernel

- **Gates run first.** Space checks, risk grading and human confirmation are
  applied by the pipeline before any handler runs; a handler inherits the same
  envelope as a built-in tool.
- **Built-in tools cannot be shadowed.** Registering a handler named
  `git/run/file/...` is rejected, and rebinding an existing contract is rejected.
- **Assembly is fail-closed.** `adapter.Apply` rejects a nil/empty adapter, a
  missing customer id, a duplicate tool, a nil handler, or an invalid contract,
  so a broken integration fails at startup rather than at the customer's first
  request.
- **Handlers are bounded.** A handler that overruns the executor timeout returns
  a timeout receipt instead of hanging the run (network handlers should still
  enforce their own request deadline).
- **Credentials and endpoints.** Handlers read credentials from env/config,
  never from source. Public adapters contain no keys, internal addresses or
  customer data; customer adapters live in private repositories.
- **Registration path.** Adapter tools are bound in memory at assembly
  (`Registry.BindContract`); they do not use the voice-driven, persisted
  `REGISTER_TOOL` flow (`Registry.Register`).

## 5. Models and data

- **Models** are selected through `model-center.json`: a local channel for
  private/local models and an external gateway for stronger hosted models, with
  tiering and fallback. A1 consolidates the remaining call sites behind one
  modelcenter facade.
- **Boundaries and red lines** (allowed spaces, test-only recipients, languages)
  are data-driven via `ConfigDir` (`spaces/*.json`, `constitution.json`), so they
  change per customer without kernel edits.

## 6. Onboarding a new customer

1. Copy `adapter.ExampleAdapter` into the customer's private integration
   package/repository.
2. Fill `Meta` (identity, language, timezone, config dir).
3. For each business action, declare a `ToolContract` (honest caps/params/risk/
   spaces) and implement a `ToolHandler` that performs the action and reads
   credentials only from env/config.
4. Record real, desensitized voice scenarios into `AcceptanceCases`.
5. At startup, after `buildOptions`, call `adapter.Apply(opts, yourAdapter{})`;
   start the service and confirm every tool is reachable through the gated path.

The kernel binary and its CI do not change between customers.

## 7. Benchmark and the 99% target (A4)

`AcceptanceCases` are replayed offline and scored on two axes per case:

- **Understanding rate** — correct intent and correct tool/parameter selection.
- **Action rate** — the gated plan runs to an end-to-end OK receipt (no
  "heard but did not act", no false success).

Cases should cover normal requests, long multi-step tasks, English and Chinese,
cancellations/corrections, and boundary/refusal scenarios. The benchmark, not a
subjective demo, is the basis for the ≥99% understanding/action target.

## 8. Roadmap

- **A0 (this document)** — adapter interface, executor handler seam, assembly
  binding, template, tests.
- **A1** — unify all model call sites behind the modelcenter facade (multi-turn,
  explicit model ids, local/external routing, error classification, health gate).
- **A2** — distill the first customer's email/attachment ingestion, mis-send
  filtering and watchdog into a concrete private customer toolset behind A0.
- **Email transport (v0.8)** — the kernel's standard email **client**
  capability (list/fetch full bodies and attachments/thread headers, send,
  ack, `OutboundGuard`, `GuaranteedSender`, and the chase/reminder tracker) is
  specified in [`mail-client-integration.md`](mail-client-integration.md). The
  kernel is a mail client, **not** an MTA; IMAP/SMTP/Graph drivers are customer
  adapters that implement `mail.Mailbox`, and credentials stay in the adapter.
- **A3** — behavior evidence gate: a run only reports success on verified
  post-conditions, never on a tool returning without error.
- **A4** — the replayable acceptance benchmark and per-deployment scorecard.
