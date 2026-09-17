# Guided `timeshare init` wizard — design spec

**Date:** 2026-09-17
**Status:** approved (design phase), pre-implementation

## Problem

`timeshare init` today is entirely flag-driven: `--vault`, `--mode`, `--ttl`, `--item`, `--move-from`, `--move-item`, `--force`. A first-time user has to already know the full flag surface before they can run it successfully — there's no discoverable, guided path, and the tool's primary entrypoint offers no help getting started. Live testing surfaced this directly: onboarding friction on the very first command a new user runs.

## Goals

- `timeshare init` becomes a guided, directed wizard by default for anyone who doesn't already know the flags.
- The full flag-driven interface keeps working unchanged for scripts/CI/power users — this is additive, not a breaking change.
- Re-running `init` in a repo that already has a `.timeshare.yml` gets a useful summary + targeted actions instead of a blank re-ask or a hard error.
- Smart defaults reduce keystrokes on the common case without guessing at anything risky.

## Non-goals (v1)

- No full TUI/REPL — this is a directed, linear wizard, not a general interactive shell.
- No scanning `.env`/`.env.example` files for item-name suggestions (considered, deferred — see Future Work).
- No changes to `read`, `run`, `status`, `lock`, `doctor`, or `token` — this spec is `init`-only.

## Entry modes

`timeshare init`'s behavior branches on flags, in this order:

1. **`--non-interactive` (`-n`) passed** → today's exact behavior unchanged. Validates the given flag combination, hard-errors on anything incomplete (`at least one of --move-from, --item, or --move-item is required`), never prompts. This is the flag for scripts/CI that must never block on stdin.
2. **No `--non-interactive`, `.timeshare.yml` already exists in the target directory** → summary + menu mode (see below), regardless of what other flags were passed.
3. **No `--non-interactive`, no existing config, zero flags given** → full wizard, blank, vault-name step pre-filled from the current directory's name.
4. **No `--non-interactive`, no existing config, some flags given** → wizard, pre-seeded: any step whose flag was already supplied is marked complete with that value and skipped; the wizard resumes at the first unanswered step.

## Components

**`internal/cli/wizard.go`** (new) — the interactive step-tracker view, built on `huh`/`lipgloss`, matching the approved layout:

- Two-column block per step: left column is a persistent vertical list of all steps (vault name, auth mode, items, TTL), right column is the active step's prompt.
- A completed step shows a checkmark, its label, and its captured value inline in a distinct color (amber) — e.g. `✓ Vault name  project-x-secrets`.
- The active step's right-column panel has a dashed-divider hint line at the bottom: `Press ? for help · <context keys> · enter to confirm`. `?` toggles a static help block in place (per-step help text, no network call, dismiss returns to the prompt).
- The items step reuses the existing dual-list picker from `pickItems` (source vault contents left, "selected so far" right) — this is the same component already shipped for `--move-item`'s interactive form, not a new implementation.
- Each completed step's block stays visible as the wizard proceeds (matches the approved mockup: the transcript scrolls, it doesn't clear).

**`internal/cli/init.go`** — `newInitCmd`:

- New flags: `--non-interactive`/`-n` (bool). Short forms added to existing flags: `-v` (`--vault`), `-m` (`--mode`), `-t` (`--ttl`), `-i` (`--item`), `-f` (`--force`). `--move-from`/`--move-item` keep long-form only — more consequential, less frequent, not worth a collision-prone single letter.
- `RunE` builds a `wizardState` from whatever flags were passed (see Data flow), then dispatches per Entry modes above.
- The existing non-interactive execution path (vault creation, `--move-from`/`--move-item` handling, `writeTimeshareConfig`) is refactored to take a `wizardState` as input instead of reading the closure-captured flag variables directly, so both the flag path and the wizard path funnel through the same execution logic — no duplicated vault-creation/move code between the two modes.

**Existing-config summary + menu mode:**

- Reads and displays the current `.timeshare.yml` (vault, mode, TTL, item count).
- Menu (`huh.NewSelect`): **Add items** (re-enters the items step only, reusing the dual-list picker against the configured vault as destination), **Change TTL** (single prompt, rewrites just that field), **Start over** (re-enters the full wizard from step 1, equivalent to today's `--force` overwrite).
- Never mutates `.timeshare.yml` until a menu action is chosen and confirmed.

## Data flow

```go
type wizardState struct {
    Vault, Mode, TTL string
    Items            []string
    MoveFrom         string
    MoveItems        []string
    set              map[string]bool // which fields came from flags, pre-seeding the wizard
}
```

1. Parse flags into `wizardState`, marking `set[field] = true` for anything non-empty/non-default.
2. Branch per Entry modes.
3. Non-interactive: validate `wizardState` exactly like today's flag checks, error on gaps.
4. Existing config: load current `.timeshare.yml` into a display struct, render summary + menu, act on selection.
5. Wizard (blank or pre-seeded): for each step in order (vault name → auth mode → items → TTL), if `set[step]` is true, render it as already-complete with the seeded value; otherwise prompt, using the repo-dir-name default only for the vault-name step when nothing was seeded.
6. On wizard completion: same `CreateVault`/`MoveItem`/`writeTimeshareConfig` calls already in `init.go`, now driven by the completed `wizardState`.

## Error handling

- Wizard mode (blank or pre-seeded) never hard-fails on missing info — prompting for what's missing is the point. Only `--non-interactive` hard-errors on incompleteness.
- `?` help is local and non-destructive: static text already known to the binary, never a network call, dismissing it returns to exactly where the user was.
- The existing-config summary/menu path is read-only until a menu action is explicitly chosen; "Start over" still requires confirmation before it overwrites (mirrors today's `--force` requiring explicit intent).
- Vault creation, item moves, and config writes keep all error handling already specified in the original design spec (partial-move reporting, no secret ever written to disk, etc.) — this spec only changes how `init` collects its inputs, not what it does with them once collected.

## Testing

- `wizardState` construction from a given flag set (which fields are marked `set`) — pure logic, real unit tests, no terminal required.
- Existing-config detection and routing to summary/menu mode — unit test using a real temp `.timeshare.yml` (matches existing `init_test.go` patterns).
- Non-interactive validation logic (gap detection) — unit tests, extending the coverage already implied by today's flag-combination checks.
- The interactive rendering itself (the step-tracker view, the help overlay, the summary/menu) stays structurally untested, matching this project's established pattern for anything requiring a real terminal (`pickItems` is the precedent — reviewed and cross-compiled, never executed in CI).

## Key decisions and rationale

| Decision | Rationale |
|---|---|
| `--non-interactive`/`-n` as a distinct flag, not reusing `--force` | `--force` already means "overwrite existing config" — collapsing two meanings onto one flag would be a footgun. |
| Any-flags-without-`--non-interactive` drops into a *pre-seeded* wizard, not a plain wizard | Preserves partial power-user intent (e.g. `--vault=x` alone) while still completing the interaction instead of erroring — matches the explicit design ask. |
| Existing-config detection takes priority over blank/pre-seeded wizard | Re-running `init` against an already-configured repo should never silently attempt a from-scratch wizard; the summary+menu is strictly more useful. |
| Vault-name default from repo directory name, nothing else defaulted | Cheapest, safest smart default (saves a keystroke on the common case); scanning `.env` files for item-name guesses was explicitly deferred as more surface area for a wrong guess. |
| Items step reuses `pickItems`, not a new component | Already shipped, already reviewed, already tested at the unit level for its surrounding logic — no reason to build a second dual-list picker. |
| Short flags only for `-v/-m/-t/-i/-f/-n`, not `--move-from`/`--move-item` | The omitted two are more consequential (they move real secrets) and used less frequently — a typo'd single-letter collision matters more there. |

## Future work (explicitly deferred)

- Scanning `.env`/`.env.example` for item-name suggestions in the items step.
- Context-aware (not just static) `?` help content.
- A `timeshare init --dry-run` or similar preview-only mode.
