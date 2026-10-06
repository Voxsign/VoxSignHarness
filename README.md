# VoiceSign Harness

VoiceSign **orchestration / observability / verification harness**: it carries a
voice or text command from raw input all the way to a four-line receipt, through a
13-stage pipeline — cleaning → correction → intent → reference resolution →
space gate → risk grading → confirmation → execution → self-healing → independent
verification → attribution → caching → receipt.

- Pure Go standard library, **zero external dependencies**.
- Line A = `server/` (HTTP entry, task table, serial gate, confirmation bridge).
- Line B = `cmd/vhs-asr` (standalone ASR personalization service).

## Repository layout

This repository is the **server-side harness** (execution plane, observability,
verification). The client side lives in companion repositories:

- [`VoxSign-IOS`](https://github.com/Voxsign/VoxSign-IOS) — the iOS client (private).
- `VoxSign-Server` — the upstream product / cloud backend (private).

The harness decouples from the client by
[`contracts/intent-v1.schema.json`](contracts/intent-v1.schema.json) (the single
authoritative intent contract) over HTTP; neither side imports the other.
See `ARCHITECTURE.md` for the layered architecture.

## Quick start

```bash
# Build everything (pure standard library, CGO disabled)
go build ./...

# Run the Line-A server
go run . serve

# Interactive REPL
go run . repl

# Run the Line-B ASR service (:8787)
go run ./cmd/vhs-asr
```

## The bundled Chinese language pack

VoiceSign is a **Chinese voice assistant**. Its ASR and command layer ship with a
built-in Chinese language pack — pinyin syllable table, pronunciation dictionary,
lexicon, filler words, punctuation rules, and the Chinese command/intent trigger
vocabulary. This data lives under `asr/` (and the intent tables under `input/`)
and is intentionally kept in Chinese; it is the engine's recognition data,
analogous to the `zh-Hans.lproj` strings table shipped inside the iOS app. All code
comments, documentation, READMEs, and log/message strings are written in English;
only this recognition data remains in Chinese by design.

## Directory overview

| Path | Purpose |
|---|---|
| `server/` | Line-A HTTP entry, task table, serial gate, confirmation bridge |
| `cmd/` | Entrypoints: `vhs-voice`, `vhs-asr`, `vhs-relay`, `vhs-ledger` |
| `asr/` | ASR engine + Chinese language pack (pinyin/dictionary/lexicon/triggers) |
| `input/` | Text cleaning, correction, intent classification, slot extraction |
| `pipeline/` | The 13-stage orchestration |
| `route/`, `router/` | Routing and ledger |
| `risk/`, `selfheal/`, `verify/` | Risk grading, self-healing, independent verification |
| `tools/`, `skill/`, `skills/` | Executor tool registry and skill system |
| `data/spaces/` | Sample space (domain) manifest definitions |
| `contracts/` | Authoritative intent schema |

## Tests

```bash
go test ./...
```

Many packages also ship `*_criteria_test.go` (red-first, mechanically checked
acceptance criteria).

## License

See repository license.
