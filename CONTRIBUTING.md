# Contributing

## Local development

```sh
make build           # builds ./bin/timeshare and ./bin/timesharedd
make check            # fmt-check, vet, golangci-lint, go test -race
make hooks-install   # installs lefthook pre-commit/pre-push hooks that
                     # mirror the same gates CI runs
```

Requires [lefthook](https://github.com/evilmartians/lefthook) and
[golangci-lint](https://golangci-lint.run/) installed locally for the full
gate set. CI runs the same checks via GitHub Actions on every PR
(`.github/workflows/ci.yml`), and `main` is protected — PRs must pass CI
before merge, and a PR's branch must be up to date with `main` before it's
mergeable (branch protection's `strict` status-check setting).

`make hooks-install` wires the gates into git hooks (`lefthook.yml`):

- **pre-commit:** `fmt-check`, `vet`, `lint` — fast, run on every commit
- **pre-push:** the full `go test -race` suite — slower, deferred to push

## Workflow

- One PR per change — this project ships small, reviewable diffs rather
  than bundling unrelated fixes.
- `go vet`/`gofmt`/`golangci-lint` findings get fixed for real when cheap;
  a by-design finding (e.g. `gosec` flagging an intentional subprocess
  exec that's the whole point of the tool) gets an inline `//nolint`
  comment with a one-line justification, never a blind suppression.
- Tests exercise real behavior (real sockets, real files in `t.TempDir()`,
  a real in-memory keychain via `keyring.MockInit()`) rather than mocking
  the thing under test.

## Project layout

See the design spec and implementation plan in `docs/superpowers/specs/`
and `docs/superpowers/plans/` for the original architecture rationale —
useful background, though the code is the source of truth for current
behavior.
