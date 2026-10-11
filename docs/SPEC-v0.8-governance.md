# SPEC v0.8 — Governance (G)

Governance is the set of gates that constrain what the agent is allowed to
do: static and dynamic risk evaluation, outbound-mail policy, tenant
isolation, and model-tier routing. These gates cannot be bypassed by an
explicit confirmation.

## Contract

- Risk evaluation classifies commands by reversibility and blast radius;
  irreversible and strong-impact actions route to a human, and a guard
  downgrades after repeated approvals on the same surface while resetting
  when the target path changes.
- Outbound mail enforces a hard recipient/domain allow-list, an explicit
  do-not-send veto in English and Chinese, and a dangerous-attachment
  extension list. Confirmation never overrides these; a guard block is
  applied before any ledger entry is created.
- Tenants are isolated: an action or message for one tenant cannot confirm,
  match, or act on another tenant's records.
- Model access is tiered. The registry enables only configured channels,
  requires an environment key for new channels, records every model
  substitution (and never reports one that did not happen), and resolves a
  tier to a fixed channel set. Learn/write-back paths are single-writer.

## Pinned validators

- `TestEvaluate_IrreversibleHuman`
- `TestEvaluate_StrongHigh`
- `TestGuard_DowngradeAfterThree`
- `TestGuard_ResetsOnDifferentPath`
- `TestGuardDoNotSend`
- `TestGuardRecipientScope`
- `TestGuardDangerousAttachment`
- `TestGuaranteedGuardBlocksBeforeLedger`
- `TestTenantIsolation`
- `TestUnrelatedSentItemDoesNotConfirm`
- `TestTiersAndChannelResolution`
- `TestNewRegistryRequiresEnvKey`
- `TestModelSubstitutionIsRecorded`
- `TestValidateC2OnlyLearnWritesBack`
