# timeshare — design spec

**Date:** 2026-09-16
**Status:** approved (design phase), pre-implementation

## Problem

Scripts and AI-agent workflows that read secrets from 1Password via `op` in a loop trigger a biometric approval prompt on every single read — confirmed by 1Password staff on their own community forum as a known, unresolved bug (internal ref dev/b5/op#2577). 1Password's official mitigation is to switch to service accounts (headless, token-based, no prompts at all), which solves prompt fatigue but drops the interactive/biometric UX entirely and gives no finer-grained access control than whole-vault, set immutably at creation.

No existing open-source tool combines: project/repo-scoped access, a local TTL cache specifically aimed at killing repeat *approval* prompts (not just re-auth), and native 1Password support. Closest prior art is the `gpg-agent`/`ssh-agent` pattern (cache the unlock, not the secret, TTL-expire it) — validated as the right model, but no maintained tool applies it to 1Password with repo scoping. A handful of tiny single-maintainer projects (`op-cache` variants, `op-fast`) attempt the 1Password-specific piece but have 0–162 GitHub stars, no repo-scoping, and mixed maintenance.

## Goals

- Kill repeat approval prompts for both the biometric desktop flow and (for consistency/auditability) service-account flow.
- Repo-scoped: each project gets its own vault binding, TTL, and item allow-list.
- Local, encrypted-at-rest where persisted; in-memory-only for v1.
- Global default TTL + per-invocation override.
- Blast-radius minimization layered on top of 1Password's own vault-level grant: an item-level allow-list enforced by timeshare itself, since 1Password service-account grants are immutable and vault-level only.

## Non-goals (v1)

- Multi-backend support beyond 1Password (interface stubbed, not implemented).
- Persisted/disk-backed cache (deferred — see Future Work).
- Windows support (Unix socket IPC assumed; revisit if demand appears).
- Team/multi-user shared daemon (one daemon per OS user).

## Architecture

```
┌─────────────┐   spawns if dead   ┌──────────────────┐
│ timeshare    │ ─────────────────▶│  timesharedd       │
│ CLI (Cobra)  │                    │  (daemon, per-user)│
│ read/run/    │◀──Unix socket─────▶│  in-mem TTL cache  │
│ init/status/ │   0600, SO_PEERCRED│  backend interface │
│ lock/doctor  │   verified          └─────────┬──────────┘
└─────────────┘                                │
                                     ┌──────────┴──────────┐
                                     │  1Password backend    │
                                     │  Mode A: service-acct  │
                                     │  Mode B: biometric CLI │
                                     └────────────────────────┘
```

One daemon per OS user, not per-project. Cache entries keyed by `(project_id, secret_name)` where `project_id` is derived from the repo's absolute path, so one daemon safely serves every repo on the machine without cross-project leakage. Socket at `$XDG_RUNTIME_DIR/timeshare/agent.sock` (Linux; platform-equivalent path elsewhere), created `0600`, daemon verifies peer UID via `SO_PEERCRED`/`LOCAL_PEERCRED` before serving any request.

CLI is a thin client: checks socket liveness, spawns daemon (detached) if absent, sends request, prints/execs result.

## Components

**`timeshare` (CLI, Cobra-based)**
- `timeshare init` — scaffolds a dedicated 1Password vault + service account for the current repo, writes `.timeshare.yml`, walks existing secrets into the new vault via `op item move` (verified: no duplicate-item warning triggered — that heuristic lives in the browser-extension save flow, not CLI vault mutations).
- `timeshare read <name>` — resolves one secret, prints to stdout. Drop-in for `op read` in scripts.
- `timeshare run -- <cmd>` — resolves everything listed in `.timeshare.yml`, injects as env vars, execs subprocess.
- `timeshare status` — inspect cache TTLs for current project.
- `timeshare lock` — force-evict current project's cache entries.
- `timeshare doctor` — health check: daemon reachable, `op` CLI present and correct version, service-account token valid, socket permissions sane, biometric session state.

Interactive flows (`init`, `doctor`) and general CLI styling/alerts/loading states use `charmbracelet/huh` (forms) and `charmbracelet/lipgloss` (styling) as Go libraries — not the standalone `gum` binary, to avoid shelling out from a security-sensitive process.

**`timesharedd` (daemon)**
- In-memory cache: `project_id → secret_name → {value, expires_at}`.
- Holds backend auth state (service-account token reference; biometric session handle for Mode B).
- Enforces item-level allow-list from `.timeshare.yml` independent of the underlying vault's actual contents — refuses anything not explicitly named, even if the vault grant would permit reading it.
- Idle-timeout self-exit after no requests for a configurable period; next CLI call respawns.

**Backend interface**
```go
type Backend interface {
    Resolve(ctx context.Context, projectCfg ProjectConfig, secretName string) (value string, ttl time.Duration, err error)
}
```
- `OnePasswordServiceAccount` (Mode A): 1Password Go SDK, token from OS keychain/env, no interactive step.
- `OnePasswordBiometric` (Mode B): shells to `op` CLI; first call may trigger biometric prompt; daemon caches the resulting unlock the way `gpg-agent` caches a passphrase, TTL = config default or 1Password's own session window, whichever is shorter.

**`.timeshare.yml`** (repo root, committed to git)
```yaml
vault: project-x-secrets
mode: service-account | biometric
ttl: 4h
items:
  - DATABASE_URL
  - STRIPE_KEY
```
No credential ever lives in this file. Mode A's service-account token is resolved from OS keychain/env at daemon startup, keyed by vault name.

## Data flow

**Cold read:** CLI resolves git root → reads `.timeshare.yml` → ensures daemon running → sends `{project_id, secret_name, requested_ttl?}` → daemon checks allow-list (reject if not listed) → cache miss → backend `Resolve()` call (may prompt, Mode B only) → daemon stores `{value, expires_at}` → returns value → CLI prints/execs.

**Warm read:** same up through allow-list check, cache hit, backend never touched — no possible prompt. This is the mechanism that solves the actual problem.

**Expired read:** treated as cold read, re-fetch, re-arm TTL.

**`timeshare run -- cmd`:** resolves every name in `items:` (parallel warm/cold reads), builds env map, execs subprocess with augmented env.

**Cross-repo isolation:** `project_id` in the cache key prevents same-named items in different repos from colliding or leaking into each other's TTL window, even served by one daemon.

## Error handling

- No `.timeshare.yml`: CLI errors immediately, no daemon spawn, points at `timeshare init`.
- Item not in allow-list: rejected before any backend call — fail closed.
- Backend auth failure: typed error surfaced to caller, never cached, never silently retried.
- Stale socket (daemon crashed): connect-timeout treated as dead, socket unlinked, daemon respawned; lockfile guards against a double-spawn race.
- Daemon crash: cache is memory-only, so crash just means a cold cache on respawn — no corrupted persisted state possible.
- TTL checked via wall-clock `now > expires_at`, not a timer callback — correct across laptop sleep/resume by construction.
- Socket permissions `0600` plus `SO_PEERCRED`/`LOCAL_PEERCRED` peer-UID verification — a second local user can't connect at all, not merely fail to authenticate.
- `init`'s `op item move` step: abort on partial failure, report which items moved vs didn't, never assume success — relies on `op item move`'s own copy-then-delete atomicity rather than re-implementing it.

## Testing

- Daemon core (TTL, allow-list, peer-UID check): table-driven unit tests against a mocked `Backend`, fake clock — no real `op` calls, fast and deterministic.
- Backend implementations: integration tests behind `//go:build integration`, gated on a real `op` CLI + test service account, CI-only.
- CLI↔daemon socket protocol: end-to-end test against a real `timesharedd` + mock backend over a temp socket, driven through the actual CLI binary.
- `init` scaffolding: integration-tagged test using a scratch vault (mirrors the manual verification already performed: create vault, create item, `op item move`, confirm no duplicate warning, teardown).
- Adversarial cases get explicit tests: wrong peer UID rejected; item outside allow-list rejected even when the vault grant would permit it; expired-but-present cache entry never returned as fresh.

## Key decisions and rationale

| Decision | Rationale |
|---|---|
| Both Mode A and Mode B in v1 | User chose full scope over incremental; shared daemon/interface avoids retrofitting later. |
| Go | Static binary, matches ssh-agent/gpg-agent/`op` ecosystem conventions, fits a long-lived daemon. |
| CLI shim + `run` env-inject, no client library | Matches how scripts already call `op` — near-zero integration cost. |
| In-memory cache only (v1) | Strongest default security posture; disk persistence deferred (see Future Work) since it's not needed for the common case. |
| `.timeshare.yml` at repo root | Matches `teller`/`direnv` convention; credentials never in the file, only structure/policy. |
| Item-level allow-list enforced by timeshare, not 1Password | 1Password service-account grants are vault-level and immutable — this is the one layer of scoping 1Password structurally can't offer post-creation. |
| Auto-spawn daemon on first CLI call | ssh-agent-style zero-setup onboarding; no systemd/launchd unit required for v1. |
| Unix socket over localhost HTTP | Filesystem-permission-scoped by default (`0600`), no port-binding race, no accidental network exposure for a process holding decrypted secrets. |
| Cobra for CLI structure | Go CLI standard (`kubectl`, `gh`, `hugo`), built-in completions, fits a 6-command surface. |
| `huh`/`lipgloss` (not `gum` binary) for TUI polish | Same visual quality without shelling out to an external process from a security-sensitive Go binary. |

## Future work (explicitly deferred)

- Disk-persisted cache (OS keychain-backed) to survive daemon restart within an 8–12hr grant window, once memory-only v1 is proven. Needs its own scoped design pass (encryption-at-rest mechanism, keychain API per platform).
- Additional backends (Vault, AWS Secrets Manager) behind the existing `Backend` interface.
- systemd/launchd unit files as an opt-in alternative to auto-spawn.
- Windows support.
