# Mail Client Integration

The kernel's standard email capability for v0.8.

> **The `mail` package is an email CLIENT, not a mail system.**
> It reads and sends mail through a small adapter surface. It does **not**
> accept, relay, queue, provision, or store mail for a domain, and it never
> holds provider credentials. A real mail server (MTA/MDA — Maddy, Postfix,
> Exchange, Google, an IMAP/SMTP provider, or a self-hosted mailbox such as the
> project's own `mail.voxsign.ai`) is an **external system** the kernel talks
> to.

## 1. Where it sits

```
 application / plugin (customer-specific, lives outside the kernel repo)
 +--------------------------------------------------------------+
 | IMAP/SMTP adapter   |   Microsoft Graph adapter   |  other   |  implements mail.Mailbox
 +-----------------------------------+--------------------------+
                                      |  four small interfaces
 +-----------------------------------v--------------------------+
 | kernel mail package (customer-neutral, Go stdlib only)      |
 |  RFC822 parse/compose · OutboundGuard · GuaranteedSender     |
 |  action ledger + reconciliation · Chase Tracker             |
 +-----------------------------------+--------------------------+
                                      |  IMAP / SMTP / Graph / ...
 +-----------------------------------v--------------------------+
 | external mail server / mailbox (NOT part of this project)   |
 +--------------------------------------------------------------+
```

The kernel stays zero-dependency and customer-neutral. Concrete protocol
drivers (which third-party IMAP/Graph library to use, which tenant, which
credentials) are implemented in the **application/plugin layer** and register
as a `mail.Mailbox`. This is what lets one kernel serve many applications.

## 2. The adapter contract

A transport adapter implements four methods:

```go
type Mailbox interface {
    Lister   // List(ctx, ListOptions) ([]Envelope, error)        // metadata only
    Fetcher  // Fetch(ctx, id) (*Message, error)                  // full MIME
    Sender   // Send(ctx, *OutboundMessage) (serverMsgID, error)
    Acker    // Ack(ctx, id, AckResult) error                     // post-processing outcome
}
```

- `Message.ID` is the source-unique idempotency key (`mailbox#UID` for IMAP,
  the file name for the file inbox) and must never collide across tenants.
- `Message` carries full `TextBody`/`HTMLBody`, decoded `Attachments`, To/Cc,
  and RFC5322 threading headers (`Message-ID`, `In-ReplyTo`, `References`).
- `Ack` is at-least-once: set `AckResult{Error: ...}` to leave a message
  unacknowledged for retry; only mark `Seen`/`Delete` after successful work.
- For outbound mail, `OutboundMessage.MessageID` is the caller-chosen
  deterministic idempotency key. Retries of one logical mail reuse it so a
  provider or reconciler can collapse duplicates.

### Method mapping by transport

| Method | IMAP | Microsoft Graph | FileMailbox (reference) |
|---|---|---|---|
| `List` | `UID SEARCH` (UNSEEN/SINCE) | `GET /mailFolders/Inbox/messages` | read inbox dir |
| `Fetch` | `UID FETCH BODY.PEEK[]` | `GET /messages/{id}?$expand=attachments` | read + parse file |
| `Send` | SMTP, or `APPEND` to Sent | `POST /sendMail` or `/messages/{id}/send` | append to sent dir |
| `Ack` | `UID STORE +FLAGS (\Seen)` / `EXPUNGE` | patch `isRead` / move message | mark/rename file |

## 3. Quickstart: the zero-dependency FileMailbox

`FileMailbox` is a dependency-free reference implementation (and a local/test
backend) that exercises the same contract a real adapter must satisfy.

```go
mb := mail.NewFileMailbox("/var/lib/vhs/mailbox")

envs, err := mb.List(ctx, mail.ListOptions{OnlyUnseen: true, Limit: 20})
if err != nil { return err }

for _, e := range envs {
    msg, err := mb.Fetch(ctx, e.ID) // full text + html + decoded attachments
    if err != nil { continue }

    if err := handle(ctx, msg); err != nil {
        _ = mb.Ack(ctx, e.ID, mail.AckResult{Error: err.Error()}) // leave for retry
        continue
    }
    _ = mb.Ack(ctx, e.ID, mail.AckResult{Seen: true})
}
```

`Compose(m *OutboundMessage) ([]byte, error)` renders a standards-compliant
RFC5322 message (text/html alternatives, Cc, threading headers, base64/quoted-
printable attachments, 25&nbsp;MB cap).

## 4. Connecting a real mailbox or MTA

1. Implement `mail.Mailbox` in your application using any IMAP/SMTP or Graph
   library. The kernel imports no third-party protocol code.
2. Inject all host/account/credential values from the environment or a
   deployment-private config file. **Never commit credentials** — CI runs a
   secret scan that fails the build on keys/tokens.
3. Give each mailbox/account its own tenant label; the action ledger and chase
   tracker are all keyed by `(tenant, key)`.

Suggested adapter environment variables (the adapter defines and owns these):

```
VHS_MAIL_IMAP_HOST / VHS_MAIL_IMAP_PORT / VHS_MAIL_IMAP_USER / VHS_MAIL_IMAP_PASS
VHS_MAIL_SMTP_HOST / VHS_MAIL_SMTP_PORT / VHS_MAIL_SMTP_USER / VHS_MAIL_SMTP_PASS
VHS_MAIL_GRAPH_TENANT / VHS_MAIL_GRAPH_TOKEN   # for the Graph adapter
```

For a self-hosted MTA (the project runs Maddy behind `mail.voxsign.ai`, with an
outbound relay), the MTA exposes IMAP(S)/SMTP(S) and the adapter simply points
at those endpoints. For Microsoft Graph, the application manages the OAuth
token and uses the correct mailbox/tenant semantics (`/me/sendMail` vs. an
application identity) outside the kernel.

> The model gateway and the mail server are different systems.
> `STRATA_API_KEY` / `VHS_EMAIL_STRATA` configure the local **model** endpoint
> used to reason about mail; they are not SMTP/IMAP credentials.

## 5. Safe outbound: `OutboundGuard`

`OutboundGuard` enforces hard policy before any send (a user saying "yes" cannot
override an out-of-scope recipient):

- `AllowedDomains` / `AllowedAddresses` — recipient allow-list; anything
  outside is rejected (`ErrBlockedRecipient`).
- Explicit negation in Chinese or English ("不要发", "do not send") sets
  `SendIntent.DoNotSend` and vetoes the send (`ErrDoNotSend`).
- `BlockedAttachmentExt` denies dangerous attachment types (a default list
  applies when empty).
- No recipient at all is rejected (`ErrNoRecipient`); risky content without
  confirmation is rejected (`ErrRiskyContent`).

The guard is constructed once and handed to the guaranteed sender, which always
applies it — application code cannot bypass it on the send path.

## 6. Reliable outbound: `GuaranteedSender` + the action ledger

`GuaranteedSender` composes the action ledger (`action` package), the guard,
the transport, and an external `SentProbe` so that retries never double-send
and uncertainty never reports success.

```go
sentBox := mail.NewFileSentBox("/var/lib/vhs/mail/sent") // implements mail.Sender
ledger, err := action.OpenFileStore("/var/lib/vhs/action-log.jsonl")
if err != nil { return err }

guard := mail.NewOutboundGuard(mail.GuardPolicy{
    AllowedDomains:   []string{"example.com"},
    AllowedAddresses: []string{"ops@example.com"},
})
sender := mail.NewGuaranteedSender(
    sentBox, ledger, "worker-1", guard, mail.NewFileSentProbe(sentBox),
)

m := &mail.OutboundMessage{
    From: "agent@example.com", To: []string{"ops@example.com"},
    Subject: "Deploy finished", TextBody: "build 42 is green.",
    MessageID: "deploy-42-notice", // deterministic idempotency key
}
res, err := sender.Send(ctx, "tenant-a", m, mail.SendIntent{Confirmed: true})
```

State semantics (see `action`):

- **SUCCEEDED** only after a non-empty, confirmed server acceptance (or a strong
  external probe). A ledger write failure can never produce a false success —
  the result stays **UNKNOWN**.
- **PENDING / in-flight** is reported as in-flight: it is neither executed again
  nor marked done. Owner + lease fencing (and a version CAS) stops two workers
  from running one side effect; a worker that loses its lease cannot overwrite
  the new owner.
- A permanently rejected send (`mail.AsRejected(err)`) becomes a retryable
  **FAILED**; timeouts/connection errors become **UNKNOWN**, the lease is
  released, and the action goes to reconciliation.
- `Reconcile` consults the `SentProbe`. Only strong evidence yields
  `ProbeConfirmedNotDone` (the one path that replays); an incomplete or delayed
  sent-folder scan yields `ProbeUnknown` and is **not** blindly re-sent.

Deterministic `Message-ID`s plus the ledger give at-least-once delivery with
duplicate collapse. Unconditional exactly-once is not promised for an external
side effect — that requires a business idempotency key, the action record, and
uncertain-result reconciliation, which this pipeline provides.

## 7. Chase / reminders and escalation (new in v0.8)

The `Tracker` drives a threaded follow-up state machine so the agent can be
asked to "chase this until it's done":

```
dispatched → acknowledged → in_progress → done
                                   ↘→ cancelled
```

`ChasePolicy` defines the SLA: `AckTimeout`/`DoneTimeout` nudge the responsible
owner, `EscalateAfter` escalates to an operator (escalation wins over a nudge),
`Cooldown` prevents spam, and `MaxNudges` caps reminders. Replies observed via
`Observe` advance the stage and reset timers; terminal states never regress; a
failed reminder does not advance and is retried on a later `Tick`.

```go
tr := mail.NewTracker(mail.NewMemoryChaseStore(), actioner, mail.ChasePolicy{}) // zero = defaults
_, _ = tr.OpenChase(ctx, "tenant-a", rootMessageID, "owner@example.com", "ops@example.com")

nudges, escalations, err := tr.Tick(ctx) // run on a schedule

_, _ = tr.Observe(ctx, "tenant-a", rootMessageID, mail.StageAcknowledged) // on reply
```

The `ChaseActioner` sends the actual mail and should route through
`GuaranteedSender` with deterministic ids `<key>.nudge<N>` and `<key>.escalate`.
Combined with optimistic-concurrency CAS on the store, this guarantees that
even with many replicas ticking at once, each nudge/escalation is sent once
(covered by a 20-replica concurrency test).

## 8. Configuration reference

| Variable | Purpose | Default |
|---|---|---|
| `VHS_EMAIL_TENANT` | default tenant label for the email list client | `default` |
| `VHS_STRATA_MODEL` | local model id used to reason about mail | `local-default` |
| `STRATA_API_KEY` | API key for the local model gateway | unset (calls fail fast) |
| `VHS_EMAIL_STRATA` | base URL of the local model gateway | `http://127.0.0.1:8080` |
| `VHS_MAIL_*` | transport host/credentials, defined by your adapter | — |

Concrete deployment values (private hostnames, model/quantization tags,
credentials) are injected at deploy time and are never committed.

## 9. Verified scope and limitations (honest)

Verified offline with `go test -race` in this release:

- RFC822 parse/compose round-trips (text+html, Cc, threading, binary and
  non-ASCII attachment names, 25&nbsp;MB cap, CRLF/LF handling).
- FileMailbox at-least-once ack and strict sent-box `Contains` (including
  cross-tenant / wrong-recipient negative cases).
- Guard rejections (scope, explicit negation, dangerous attachments, no
  recipient).
- GuaranteedSender idempotency, owner/lease fencing, same-worker concurrency
  (16 goroutines → one execution), ledger-write-failure → UNKNOWN, and
  reconciliation semantics; mutation tests capture each fault injection.
- Tracker SLA decisions, reply-driven stage transitions, terminality, failed-
  send retry, and exactly-one nudge across 20 concurrent replicas.

Not included / not claimed in v0.8: production IMAP/SMTP/Graph drivers (they
live in the application adapter), real MTA provisioning or deployment, real
message delivery, and database migrations. See
`eval/tasks/email-send-once/verify.md` for the verification contract.

## 10. See also

- `docs/adapter-contract.md` — how an application registers customer-specific
  capabilities on the customer-neutral kernel.
- `docs/SPEC-v0.8-verification.md`, `docs/SPEC-v0.8-governance.md` — the
  verification and governance layers these components satisfy.
