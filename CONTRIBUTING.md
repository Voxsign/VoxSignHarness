# Contributing to VoxSignHarness

Thank you for contributing! VoxSignHarness is fully open source under the
[Apache License 2.0](LICENSE) — personal and commercial use are both welcome.

## Ways to contribute

- **Report a bug or request a feature** — open an Issue. Include the command
  or flow that failed, expected vs actual behavior, and relevant logs.
- **Submit code** — follow the Pull Request flow below.
- **Improve documentation** — fixes and clarifications to README /
  ARCHITECTURE are always appreciated.

## Pull Request flow

1. Fork the repository and create a feature branch:
   `git checkout -b feat/your-change`
2. Make your changes, following the [Code style](#code-style) section.
3. Add tests for new behavior (Go table tests preferred). Run them locally:
   `go test ./...` (note: `asr` and `doccontract` packages require private
   data and are expected to fail in the public repo; the CI pipeline excludes
   them).
4. Commit **with the DCO sign-off (mandatory)**:
   `git commit -s -m "feat: describe your change"`
   This adds `Signed-off-by: Your Name <your@email>` to the commit — it
   certifies you are legally entitled to contribute this code.
5. Push and open a Pull Request against `main`.
6. CI must pass (build + vet + tests + sensitive-info scan). A maintainer
   reviews the PR; **only maintainers can merge**.

## Review process — three gates

1. **Automated CI** — `go build ./...`, `go vet ./...`, unit tests, and a
   scan for secrets / personal identifiers. Must be green before merge.
2. **Code review** — a maintainer (with AI-assisted review) checks logic,
   quality, and scope.
3. **Maintainer merge** — the project owner gives final approval and merges.

## Code style

- `gofmt` clean, idiomatic Go; follow the conventions of the existing
  packages.
- **No personal names, real emails, credentials, API keys, or internal host
  details in code, comments, configs, or docs** — the CI scan rejects them.
- New behavior must include tests; run `go test` on affected packages before
  pushing.

## Protected paths

Paths listed in `CODEOWNERS` (core engine packages: `asr`, `recog`,
`pipeline`, `contracts`, `provider`) require explicit approval from the
project owner. Changes touching them cannot be merged without owner approval.

## Code of conduct

Be respectful and constructive. Harassment, trolling, or spam will result in
removal from the project. No exceptions.

## Questions

Open a discussion in Issues, or comment on an existing Issue/PR. We reply in
English.
