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
- Every PR gets a complexity pass before merge, on top of correctness/
  security review — flag unused flexibility, hand-rolled stdlib
  equivalents, and single-implementation abstractions, and cut them
  before the diff lands. There's no automated CI gate for this (it needs
  a reviewer making a judgment call, not a lint rule), so it's a manual
  step in the PR process, not a GitHub Actions job.

## Releasing

Versions follow [semver](https://semver.org/) as annotated git tags
`vMAJOR.MINOR.PATCH` (e.g. `v0.1.3`). While the project is early /
pre-1.0, breaking changes may land in minor bumps; once `v1.0.0` ships,
breaking changes require a major bump.

Cutting a release (from an up-to-date `main`, after CI is green):

```sh
git checkout main && git pull
git tag -a v0.1.3 -m "timeshare v0.1.3"
git push origin v0.1.3
```

Pushing the tag runs `.github/workflows/release.yml` (GoReleaser), which
builds linux/darwin amd64+arm64 binaries for `timeshare` and `timesharedd`,
attaches them to a GitHub Release, embeds the tag/commit/date into
`--version` output via ldflags, and updates the Homebrew cask in
[`newtosh/homebrew-tap`](https://github.com/newtosh/homebrew-tap).

### Homebrew tap token (one-time)

GoReleaser pushes the cask into a **separate** repo, so the default
`GITHUB_TOKEN` is not enough. Create a fine-grained personal access token:

1. Open this pre-filled form (Resource owner: **newtosh**, Contents: **Read
   and write**):
   [Create fine-grained PAT](https://github.com/settings/personal-access-tokens/new?name=timeshare-homebrew-tap&description=GoReleaser+pushes+Casks%2Ftimeshare.rb+into+newtosh%2Fhomebrew-tap&target_name=newtosh&expires_in=366&contents=write)
2. Under **Repository access**, choose **Only select repositories** →
   `homebrew-tap` only.
3. Generate the token, then:

```sh
gh secret set HOMEBREW_TAP_TOKEN -R newtosh/timeshare
```

Without this secret, the GitHub Release still publishes; only the tap
update is skipped/fails.

Do not retag or force-push an existing version tag — publish the next
patch (e.g. `v0.1.4`) instead.

## Project layout

See the design spec and implementation plan in `docs/superpowers/specs/`
and `docs/superpowers/plans/` for the original architecture rationale —
useful background, though the code is the source of truth for current
behavior.
