# Time-boxed SSH key access via timeshare — design spec

**Date:** 2026-09-18
**Status:** approved (design phase), pre-implementation

## Problem

1Password already manages SSH keys and already provides a system SSH agent (`~/.1password/agent.sock`) with its own biometric-unlock and session-timeout policy — confirmed working end to end in prior macOS field testing. That agent, though, exposes *every* key loaded in the user's 1Password account to *anything* that talks to its socket, and its session lifetime is a single app-wide setting, not something a given repo or script can scope down on its own.

timeshare already solves the equivalent problem for plain secrets: a repo declares an allow-list (`.timeshare.yml`'s `items`) and a TTL, and `timeshare run`/`read` enforce both. This spec extends the same blast-radius-scoping and TTL-bounding model to SSH keys, without re-implementing or fighting 1Password's own agent.

## Goals

- A repo can declare which SSH key(s) (already stored in 1Password) it's allowed to use, the same way it declares which secret items it's allowed to read.
- Access to those keys is bounded by the repo's own TTL, independent of 1Password's app-wide SSH agent session policy.
- Private key material never enters timeshare's process at any point — signing stays 1Password's job.
- No change to normal, non-timeshare SSH/git usage on the machine.

## Non-goals (v1)

- Not a system-wide SSH agent replacement — no persistent, always-on, multi-repo multiplexing daemon. Considered and rejected: bigger blast radius (one process now mediates every repo's SSH access, including repos with no `.timeshare.yml` opinion at all) for no meaningful resource savings over the per-invocation model (see Constraints).
- No SSH key generation, rotation, or management — timeshare only filters access to keys 1Password already holds.
- No fix for OpenSSH's `IdentityAgent`-over-`SSH_AUTH_SOCK` config precedence — documented workaround only (see below). timeshare's subprocess runs after SSH's own config resolution; it cannot intercept that.
- No changes to `read`, `status`, `lock`, `doctor`'s core behavior beyond one new `doctor` check (see CLI surface) — this spec is scoped to `init` and `run`.

## Why a filtering proxy, not a key-holding agent

1Password-generated SSH keys are non-exportable by design — the CLI has no path to raw private key bytes, only to a public key and to signing via the agent protocol. This isn't a constraint timeshare is choosing; it's the only mechanism 1Password exposes, and it's the right one: it means the only viable design is a **pass-through filtering proxy**, not a cache of key material. This is a stronger security property than timeshare's existing secret-value cache, not a weaker one — there is no key state to leak, persist, or accidentally write to disk.

## Architecture

`timeshare run` spawns a short-lived SSH agent (implements the `ssh-agent` wire protocol via `golang.org/x/crypto/ssh/agent`) scoped to just that subprocess invocation — same lifecycle as today's env-var injection, not a separate daemon process.

The proxy:

1. Connects to the real 1Password agent (`~/.1password/agent.sock`) as an upstream client.
2. On startup, resolves each `.timeshare.yml` `ssh_keys` entry (a `<vault>/<item>` reference — SSH keys stay wherever they already live in 1Password, they are not copied into the repo's own dedicated vault the way secret `items` are) to a fingerprint via `op item get` (one `op` call per entry, once, before the listener starts accepting).
3. On `REQUEST_IDENTITIES`, forwards upstream, then filters the response down to only allow-listed fingerprints before replying to the client.
4. On `SIGN_REQUEST`, checks the target key's fingerprint against the allow-list AND the run's TTL deadline. Both pass → forwards the raw signing request upstream and relays the signature back untouched, no inspection of its contents. Either check fails → replies `SSH_AGENT_FAILURE`.

No `Add()`/`Remove()` support — read-only passthrough; timeshare never manages what keys exist in 1Password, only which already-loaded ones a given `run` invocation may reach.

## Components

- **`internal/sshagent/proxy.go`** (new package) — `Proxy` type wrapping an `agent.Agent`-compatible server: holds an upstream `agent.ExtendedAgent` client, an allow-listed fingerprint set, and a TTL deadline. Implements `List()` and `Sign()`; `Add()`/`Remove()`/`Lock()`/`Unlock()` return "not supported."
- **`internal/sshagent/resolve.go`** (new) — resolves `.timeshare.yml` `ssh_keys` entries, each a `<vault>/<item-title-or-id>` reference (same split-on-rightmost-`/` syntax `--from-item` already uses — see `internal/cli/init.go`'s handling of `s.FromItems`), to fingerprints via `op item get <ref> --vault=<vault> --fields label=fingerprint --format=json`. Verified live: an SSH Key category item exposes this field directly (no `--reveal` needed — it's not a concealed field), already formatted as `SHA256:<base64>`, the exact format `golang.org/x/crypto/ssh`'s own `ssh.FingerprintSHA256` produces — no reformatting needed to compare against identities the upstream agent returns. Fails fast, with the same "did you mean" suggestion pattern `printSuggestions` already uses for items, if an entry doesn't resolve or isn't an SSH Key category item.
- **`internal/cli/run.go`** (existing `run` command) — when `.timeshare.yml`'s `ssh_keys` is non-empty: before exec'ing the wrapped command, resolve fingerprints, start the proxy on a temp socket (`0600`, `$XDG_RUNTIME_DIR/timeshare-ssh-<pid>-<nanos>.sock`), set `SSH_AUTH_SOCK` in the child's env alongside the existing resolved secret env vars. On subprocess exit (including signal-driven exit), close the listener and remove the socket file unconditionally (`defer`).
- **`internal/config/config.go`** — `Config` gains `SSHKeys []string` (`yaml:"ssh_keys"`), optional (zero value is a valid, common case — most repos won't use this). `rawConfig` and `Load`'s validation mirror `Items`'s existing handling, except an empty `ssh_keys` is not an error (unlike the existing "must list at least one item" rule for `items`).

## Data flow (per `timeshare run`)

1. Load `.timeshare.yml`.
2. If `ssh_keys` is empty: unchanged behavior, no proxy, no `SSH_AUTH_SOCK` override.
3. If non-empty: resolve each entry to a fingerprint (fails the whole `run` before anything execs if any entry is bad — same fail-fast posture as an unresolvable secret item today).
4. Start the proxy, deadline = `time.Now().Add(cfg.TTL)`.
5. Exec the target command with `SSH_AUTH_SOCK=<temp-socket>` merged into its env.
6. On exit: tear down the listener and socket file. Nothing outlives the wrapped process.

## Config schema

```yaml
vault: my-project-secrets
mode: biometric
ttl: 4h
items:
  - DATABASE_URL
ssh_keys:
  - Private/deploy-key-prod
```

`ssh_keys` entries are `<vault>/<item-title-or-id>` references, validated to be an SSH Key category item — at `run` time, not at `config.Load` time. Unlike `items`, these are deliberately **not** scoped to `vault:` (the repo's own dedicated vault) — SSH keys are typically personal, cross-project, and already live wherever the user keeps them in 1Password; `init` never copies or moves them anywhere, only records a reference. `Load` stays a pure parse (no `op` calls), matching its current contract; `status`/`doctor` calling `Load` today never shells out, and this doesn't change that.

## CLI / wizard surface

- **`timeshare init`**: new optional wizard step after "Items to move" — "SSH keys to grant access to." Reuses the existing two-step flow `--from-item`'s interactive picker already has (pick a vault via `pickVault`, then pick items from it), swapping in a category-filtered listing (`op item list --categories "SSH Key"`) for the second step, and stores `<vault>/<item-id>` — same construction `wizard.go` already does for `s.FromItems`. Skippable — unlike the items step, an empty selection here is valid and the norm. Flag form: `--ssh-key <vault>/<item>` (repeatable), mirroring `--from-item`'s `<vault>/<item-name-or-id>` syntax.
- **`timeshare run`**: no new flags. Reads `ssh_keys` from config automatically, exactly as it already reads `items`.
- **`timeshare doctor`**: one additional check, only run when the current repo's `ssh_keys` is non-empty — confirm `~/.1password/agent.sock` (or 1Password's configured agent path, if customized) is reachable, so a broken upstream is caught by `doctor` rather than surfacing mid-script as a confusing `run` failure.

## The `IdentityAgent` precedence gotcha

OpenSSH's per-`Host` `IdentityAgent` directive in `~/.ssh/config` takes precedence over the `SSH_AUTH_SOCK` environment variable when both apply to a given host. A user with `IdentityAgent ~/.1password/agent.sock` set for a host will have `ssh`/`git` ignore `run`'s `SSH_AUTH_SOCK` override entirely for that host, silently falling through to the *unfiltered* real agent — the opposite of what this feature is for.

timeshare cannot fix this from inside `run`: SSH resolves its own config before the wrapped subprocess starts, and there is no supported way for a parent process to override a child's SSH client config short of also controlling `GIT_SSH_COMMAND` or per-invocation `-o` flags, which timeshare can't inject into an arbitrary wrapped command. Handling is documentation, not code:

- `run` still sets `SSH_AUTH_SOCK` — correct today for any user without a competing `IdentityAgent` override, and it's what git's SSH transport falls back to absent one.
- The README documents the gotcha directly: users with a per-host `IdentityAgent` override need either a `GIT_SSH_COMMAND='ssh -o IdentityAgent="$SSH_AUTH_SOCK"'` in the wrapped command's own env, or an `ssh_config` `Host` pattern scoped to exclude repos meant to go through timeshare's proxy.
- `timeshare doctor`'s new check (above) does **not** attempt to detect this misconfiguration — inspecting `~/.ssh/config` for `Host`-pattern matches against arbitrary future remotes is real complexity for a non-fatal footgun with a documented fix; out of scope for v1.

## Constraints — resource impact of concurrent repos

No new OS process is created per repo. The proxy is a goroutine plus one `net.Listener` inside the `timeshare run` process that's already running to exec/wait on the wrapped command. N repos running `timeshare run` concurrently means N processes that already exist, each with one extra goroutine (a few KB of stack) and one extra Unix socket fd — not N new daemons.

No key material is cached, so per-run state is trivial: a small fingerprint set plus a deadline. The listener is idle (blocked on `Accept`/`Read`) except during an actual sign, which is one round-trip to the single system-wide 1Password agent process — the same process every `ssh`/`git` invocation on the machine already talks to today, proxy or not. Concurrency scales with how many `timeshare run` processes a user actually has open, which is already bounded by their own terminal/script usage; this is not a new resource pressure the design introduces.

## Error handling

- Upstream 1Password agent unreachable at proxy startup → `run` fails fast before exec'ing, same posture as an unresolvable secret today.
- A `ssh_keys` entry doesn't resolve, or resolves to a non-SSH-Key-category item → fails fast, names the bad entry with suggestions.
- TTL expiry mid-run → the proxy starts returning `SSH_AGENT_FAILURE` for all further requests; the wrapped subprocess is not killed. Matches the existing "the grant lapses, the process doesn't" behavior of secret TTLs.
- A `SIGN_REQUEST` for a fingerprint outside the allow-list → `SSH_AGENT_FAILURE`, plus a stderr log line naming the rejected fingerprint, so an unexpected `git push` failure is diagnosable instead of silently confusing.

## Testing

- `internal/sshagent`: unit tests against a fake upstream `agent.Agent` — `golang.org/x/crypto/ssh/agent`'s own `agent.NewKeyring()` serves as an in-memory upstream loaded with test keys, no real 1Password process needed. Cover: filtering (`List()` only returns allow-listed identities), TTL cutoff (`Sign()` fails once the deadline passes even for an allow-listed key), and rejection (`Sign()` for a non-allow-listed fingerprint never reaches the fake upstream).
- No integration test against the real 1Password agent in CI, matching the existing `//go:build integration`-gated tests in `internal/onepassword` — needs a live signed-in account and real SSH Key items, not something CI can provision.

## Future work (explicitly deferred, not v1)

- `timeshare status` surfacing SSH key grants alongside cached secrets (blocked on the same "cache has no enumeration method" gap already tracked for secrets).
- `doctor` detecting an `IdentityAgent` override that would shadow `run`'s `SSH_AUTH_SOCK`, if this turns out to bite users often enough to justify the complexity.
