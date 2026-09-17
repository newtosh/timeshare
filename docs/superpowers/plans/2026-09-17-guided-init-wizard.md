# Guided init wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `timeshare init` a guided, directed wizard by default (no flags), while keeping the existing flag-driven interface fully working for scripts/power users behind a new `--non-interactive`/`-n` flag.

**Architecture:** A `wizardState` struct captures which inputs came from flags (via Cobra's `Changed()` introspection) vs are still unset. Entry mode dispatch (non-interactive / existing-config summary / blank-or-seeded wizard) all funnel into one shared `runInit(cwd, *wizardState) error` execution core — the vault-creation/item-move/config-write logic that already exists today, unchanged in behavior, just decoupled from directly reading Cobra flag variables.

**Tech Stack:** Go, Cobra (flags), `charmbracelet/huh` (interactive prompts, already a dependency), `charmbracelet/lipgloss` (step-block rendering, already a dependency).

**Spec:** `docs/superpowers/specs/2026-09-17-guided-init-wizard-design.md`

## Global Constraints

- No `.timeshare.yml` write ever includes a credential — unchanged from the existing constraint, this plan only changes how inputs are collected, not what's written.
- Wizard mode (blank or pre-seeded) never hard-fails on missing info; only `--non-interactive` hard-errors on incompleteness.
- `?` help is local/static text only — never a network call.
- The existing-config summary/menu path never mutates `.timeshare.yml` until a menu action is explicitly chosen.
- Short flags: `-v`/`--vault`, `-m`/`--mode`, `-t`/`--ttl`, `-i`/`--item`, `-f`/`--force`, `-n`/`--non-interactive`. `--move-from`/`--move-item` keep long-form only.
- Interactive rendering (huh forms, the step-tracker view, help overlay, summary/menu) stays structurally untested per this project's established pattern (`pickItems` is the precedent) — reviewed and cross-compiled, not unit-tested. Pure logic underneath it (state construction, validation, rendering-to-string, config detection) gets real unit tests.

---

## Task 1: wizardState construction and non-interactive validation

**Files:**
- Create: `internal/cli/wizard_state.go`
- Create: `internal/cli/wizard_state_test.go`

**Interfaces:**
- Consumes: `github.com/spf13/cobra` (`*cobra.Command`, `.Flags().Changed(name)`), `github.com/newtosh/timeshare/internal/projectid` (`FindGitRoot`)
- Produces: `wizardState{Vault, Mode, TTL, MoveFrom string; Items, MoveItems []string; Force bool; set map[string]bool}`, `newWizardState(cmd *cobra.Command, vault, mode, ttl, moveFrom string, items, moveItems []string, force bool) *wizardState`, `(*wizardState).anyFlagsSet() bool`, `(*wizardState).validateComplete() error`, `defaultVaultName(cwd string) string`

- [ ] **Step 1: Write the failing tests**

```go
// internal/cli/wizard_state_test.go
package cli

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func newTestInitFlags() *cobra.Command {
	cmd := &cobra.Command{Use: "init", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().StringP("vault", "v", "", "")
	cmd.Flags().StringP("mode", "m", "biometric", "")
	cmd.Flags().StringP("ttl", "t", "4h", "")
	cmd.Flags().String("move-from", "", "")
	cmd.Flags().StringArrayP("item", "i", nil, "")
	cmd.Flags().StringArray("move-item", nil, "")
	cmd.Flags().BoolP("force", "f", false, "")
	cmd.Flags().BoolP("non-interactive", "n", false, "")
	return cmd
}

func TestNewWizardStateNoFlagsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "", "biometric", "4h", "", nil, nil, false)
	if s.anyFlagsSet() {
		t.Fatal("expected anyFlagsSet false when no flags were passed")
	}
}

func TestNewWizardStateSomeFlagsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{"--vault=project-x"}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "project-x", "biometric", "4h", "", nil, nil, false)
	if !s.anyFlagsSet() {
		t.Fatal("expected anyFlagsSet true when --vault was passed")
	}
	if !s.set["vault"] {
		t.Fatal("expected set[\"vault\"] true")
	}
	if s.set["mode"] {
		t.Fatal("expected set[\"mode\"] false — mode was left at its default, not explicitly passed")
	}
}

func TestNewWizardStateItemFlagsCountAsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{"--move-item=legacy-vault/DATABASE_URL"}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "", "biometric", "4h", "", nil, []string{"legacy-vault/DATABASE_URL"}, false)
	if !s.anyFlagsSet() {
		t.Fatal("expected anyFlagsSet true when --move-item was passed")
	}
	if !s.set["move-item"] {
		t.Fatal("expected set[\"move-item\"] true")
	}
}

func TestValidateCompleteRejectsMissingVault(t *testing.T) {
	s := &wizardState{TTL: "4h", Items: []string{"X"}}
	if err := s.validateComplete(); err == nil {
		t.Fatal("expected error for missing vault")
	}
}

func TestValidateCompleteRejectsNoItemSource(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "4h"}
	if err := s.validateComplete(); err == nil {
		t.Fatal("expected error when no --item/--move-from/--move-item given")
	}
}

func TestValidateCompleteRejectsBadTTL(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "not-a-duration", Items: []string{"X"}}
	if err := s.validateComplete(); err == nil {
		t.Fatal("expected error for invalid TTL")
	}
}

func TestValidateCompleteAcceptsMoveFromOnly(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "4h", MoveFrom: "legacy"}
	if err := s.validateComplete(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDefaultVaultNameFallsBackToDirName(t *testing.T) {
	// A directory with no .git ancestor: defaultVaultName falls back to the
	// base name of cwd itself rather than erroring.
	tmp := t.TempDir()
	got := defaultVaultName(tmp)
	want := filepath.Base(tmp)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run 'TestNewWizardState|TestValidateComplete|TestDefaultVaultName'`
Expected: FAIL with "undefined: newWizardState" / "undefined: wizardState" / "undefined: defaultVaultName"

- [ ] **Step 3: Implement**

```go
// internal/cli/wizard_state.go
package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/newtosh/timeshare/internal/projectid"

	"github.com/spf13/cobra"
)

// wizardState captures timeshare init's inputs and which of them came from
// explicitly-passed flags (via Cobra's Changed() introspection, since
// several flags — mode, ttl — have non-empty defaults that a plain
// zero-value check can't distinguish from an explicit pass).
type wizardState struct {
	Vault     string
	Mode      string
	TTL       string
	MoveFrom  string
	Items     []string
	MoveItems []string
	Force     bool

	set map[string]bool
}

// wizardFlagNames are the flags that count toward "the user passed
// something" for entry-mode dispatch. --force and --non-interactive are
// deliberately excluded — they modify behavior, not wizard input.
var wizardFlagNames = []string{"vault", "mode", "ttl", "item", "move-from", "move-item"}

func newWizardState(cmd *cobra.Command, vault, mode, ttl, moveFrom string, items, moveItems []string, force bool) *wizardState {
	s := &wizardState{
		Vault:     vault,
		Mode:      mode,
		TTL:       ttl,
		MoveFrom:  moveFrom,
		Items:     items,
		MoveItems: moveItems,
		Force:     force,
		set:       make(map[string]bool),
	}
	for _, name := range wizardFlagNames {
		if cmd.Flags().Changed(name) {
			s.set[name] = true
		}
	}
	return s
}

// anyFlagsSet reports whether any wizard-input flag was explicitly passed.
func (s *wizardState) anyFlagsSet() bool {
	for _, name := range wizardFlagNames {
		if s.set[name] {
			return true
		}
	}
	return false
}

// validateComplete checks the same completeness rules the original
// flag-only init enforced: required for the --non-interactive path.
func (s *wizardState) validateComplete() error {
	if s.Vault == "" {
		return fmt.Errorf("--vault is required (e.g. --vault=project-x-secrets)")
	}
	if s.MoveFrom == "" && len(s.Items) == 0 && len(s.MoveItems) == 0 {
		return fmt.Errorf("at least one of --move-from, --item, or --move-item is required (a config with an empty items list will never load)")
	}
	if _, err := time.ParseDuration(s.TTL); err != nil {
		return fmt.Errorf("invalid --ttl: %w", err)
	}
	return nil
}

// defaultVaultName suggests a vault name for the blank wizard's vault-name
// step: the repo's directory name if cwd is inside a git repo, otherwise
// cwd's own base name. Never errors — worst case it suggests something the
// user is free to overwrite.
func defaultVaultName(cwd string) string {
	root, err := projectid.FindGitRoot(cwd)
	if err != nil {
		root = cwd
	}
	return filepath.Base(root)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run 'TestNewWizardState|TestValidateComplete|TestDefaultVaultName'`
Expected: PASS (7 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/wizard_state.go internal/cli/wizard_state_test.go
git commit -m "feat: add wizardState construction and non-interactive validation"
```

---

## Task 2: Existing-config detection

**Files:**
- Create: `internal/cli/wizard_existing.go`
- Create: `internal/cli/wizard_existing_test.go`

**Interfaces:**
- Consumes: `github.com/newtosh/timeshare/internal/config` (`config.Load`, `config.Config`)
- Produces: `loadExistingConfig(cfgPath string) (cfg config.Config, exists bool, err error)`

- [ ] **Step 1: Write the failing tests**

```go
// internal/cli/wizard_existing_test.go
package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExistingConfigMissingFile(t *testing.T) {
	tmp := t.TempDir()
	cfg, exists, err := loadExistingConfig(filepath.Join(tmp, ".timeshare.yml"))
	if err != nil {
		t.Fatalf("expected no error for a missing file, got: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false for a missing file")
	}
	if cfg.Vault != "" {
		t.Fatalf("expected zero-value config, got %+v", cfg)
	}
}

func TestLoadExistingConfigRealFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".timeshare.yml")
	content := "vault: project-x\nmode: biometric\nttl: 4h\nitems:\n  - DATABASE_URL\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, exists, err := loadExistingConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}
	if cfg.Vault != "project-x" || len(cfg.Items) != 1 {
		t.Fatalf("got %+v", cfg)
	}
}

func TestLoadExistingConfigMalformedFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".timeshare.yml")
	if err := os.WriteFile(path, []byte("items: []\n"), 0o644); err != nil { // missing vault, empty items
		t.Fatal(err)
	}

	_, exists, err := loadExistingConfig(path)
	if err == nil {
		t.Fatal("expected an error for a malformed existing config")
	}
	if !exists {
		t.Fatal("expected exists=true even on parse failure — the file is there, just broken")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run TestLoadExistingConfig`
Expected: FAIL with "undefined: loadExistingConfig"

- [ ] **Step 3: Implement**

```go
// internal/cli/wizard_existing.go
package cli

import (
	"os"

	"github.com/newtosh/timeshare/internal/config"
)

// loadExistingConfig reports whether cfgPath exists and, if so, attempts to
// parse it. exists is true whenever the file is present, even if parsing
// fails — the caller needs to distinguish "no file" (blank wizard) from
// "file present but broken" (still a real error to surface).
func loadExistingConfig(cfgPath string) (config.Config, bool, error) {
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return config.Config{}, false, nil
		}
		return config.Config{}, false, statErr
	}

	cfg, err := config.Load(cfgPath)
	return cfg, true, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run TestLoadExistingConfig`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/wizard_existing.go internal/cli/wizard_existing_test.go
git commit -m "feat: add existing .timeshare.yml detection"
```

---

## Task 3: Step-block rendering (pure, testable)

**Files:**
- Create: `internal/cli/wizard_render.go`
- Create: `internal/cli/wizard_render_test.go`

**Interfaces:**
- Consumes: `github.com/charmbracelet/lipgloss`
- Produces: `wizardStep{Label, Value string; Done bool}`, `renderStepBlock(steps []wizardStep, activeIdx int, activeContent string) string`

- [ ] **Step 1: Write the failing tests**

```go
// internal/cli/wizard_render_test.go
package cli

import (
	"strings"
	"testing"
)

func TestRenderStepBlockShowsDoneStepsWithValue(t *testing.T) {
	steps := []wizardStep{
		{Label: "Vault name", Value: "project-x-secrets", Done: true},
		{Label: "Auth mode", Value: "", Done: false},
	}
	out := renderStepBlock(steps, 1, "Auth mode:")

	if !strings.Contains(out, "Vault name") || !strings.Contains(out, "project-x-secrets") {
		t.Fatalf("expected completed step label and value in output, got:\n%s", out)
	}
	if !strings.Contains(out, "✓") {
		t.Fatalf("expected a checkmark for the completed step, got:\n%s", out)
	}
}

func TestRenderStepBlockShowsActiveContent(t *testing.T) {
	steps := []wizardStep{{Label: "Vault name", Done: false}}
	out := renderStepBlock(steps, 0, "Vault name:\nproject-x-secrets_")

	if !strings.Contains(out, "project-x-secrets_") {
		t.Fatalf("expected active step content in output, got:\n%s", out)
	}
}

func TestRenderStepBlockMarksActiveStepDistinctly(t *testing.T) {
	steps := []wizardStep{
		{Label: "Vault name", Value: "x", Done: true},
		{Label: "Items", Done: false},
		{Label: "TTL", Done: false},
	}
	out := renderStepBlock(steps, 1, "Items:")

	if !strings.Contains(out, "▸") {
		t.Fatalf("expected an active-step marker (▸), got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run TestRenderStepBlock`
Expected: FAIL with "undefined: wizardStep" / "undefined: renderStepBlock"

- [ ] **Step 3: Implement**

```go
// internal/cli/wizard_render.go
package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// wizardStep is one row in the step tracker's left column.
type wizardStep struct {
	Label string
	Value string
	Done  bool
}

var (
	wizardCheckStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	wizardValueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	wizardActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	wizardDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	wizardLeftCol     = lipgloss.NewStyle().Width(30).Padding(0, 2, 0, 0)
	wizardRightCol    = lipgloss.NewStyle().PaddingLeft(2).BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(lipgloss.Color("240"))
)

// renderStepBlock renders one snapshot of the wizard: a left column listing
// every step (checkmark + captured value for done steps, an active marker
// for the current step, dimmed for the rest) beside a right column holding
// activeContent verbatim (the active step's prompt/help text — the caller
// owns what that string contains, this function only lays it out).
func renderStepBlock(steps []wizardStep, activeIdx int, activeContent string) string {
	var left strings.Builder
	for i, st := range steps {
		switch {
		case st.Done:
			left.WriteString(wizardCheckStyle.Render("✓") + " " + st.Label)
			if st.Value != "" {
				left.WriteString("  " + wizardValueStyle.Render(st.Value))
			}
			left.WriteString("\n")
		case i == activeIdx:
			left.WriteString(wizardActiveStyle.Render("▸ "+st.Label) + "\n")
		default:
			left.WriteString(wizardDimStyle.Render("  "+st.Label) + "\n")
		}
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		wizardLeftCol.Render(left.String()),
		wizardRightCol.Render(activeContent),
	)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run TestRenderStepBlock`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/wizard_render.go internal/cli/wizard_render_test.go
git commit -m "feat: add pure step-block rendering for the init wizard"
```

---

## Task 4: Extract the shared execution core

**Files:**
- Modify: `internal/cli/init.go`
- Modify: `internal/cli/init_test.go`

**Interfaces:**
- Consumes: `wizardState` (Task 1), `onepassword.CreateVault/ListItems/MoveItem/GetItem`, `config.Config`, `writeTimeshareConfig` (already exists)
- Produces: `runInit(cwd string, s *wizardState) error`

This task's job is a **behavior-preserving refactor**: extract the vault-creation/item-move/config-write logic (currently inline in `RunE`) into a standalone `runInit` function driven by `*wizardState` instead of closure-captured flag variables. `RunE` itself is not rewired to the new entry-mode dispatch yet — that's Task 7. For this task, `RunE` still does exactly what it does today, just by constructing a `wizardState` from the parsed flags and calling `runInit`, so the existing tests keep passing unchanged.

- [ ] **Step 1: Confirm the existing behavior test still describes today's contract**

Read `internal/cli/init_test.go`'s current `TestWriteTimeshareConfig` — it exercises `writeTimeshareConfig` directly and is unaffected by this refactor. No new test is written in this step; this task is verified by the existing suite continuing to pass end-to-end after the refactor (Step 3 below), since `runInit`'s only callers so far are behavior-identical to before.

- [ ] **Step 2: Run the existing suite to confirm the baseline before refactoring**

Run: `go test ./internal/cli/...`
Expected: PASS (baseline, before any change in this task)

- [ ] **Step 3: Refactor**

```go
// internal/cli/init.go — replace the whole file with this
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"

	"github.com/spf13/cobra"
)

func hourDuration() time.Duration { return time.Hour }

// printSuggestions prints up to 3 "did you mean" candidates for ref within
// sourceVault. Item titles aren't guaranteed unique, so suggestions
// include the ID for a stable follow-up reference. Silently does nothing
// if the vault itself can't be listed (the caller already has a real
// error to report).
func printSuggestions(sourceVault, ref string) {
	items, err := onepassword.ListItems(sourceVault)
	if err != nil {
		return
	}
	matches := onepassword.SuggestMatches(ref, items, 3)
	if len(matches) == 0 {
		return
	}
	fmt.Printf("%q not found in %q. Did you mean:\n", ref, sourceVault)
	for _, m := range matches {
		fmt.Printf("  - %s (id: %s)\n", m.Title, m.ID)
	}
}

func writeTimeshareConfig(path string, cfg config.Config) error {
	content := fmt.Sprintf(
		"vault: %s\nmode: %s\nttl: %s\nitems:\n",
		cfg.Vault, cfg.Mode, cfg.TTL,
	)
	for _, item := range cfg.Items {
		content += "  - " + item + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // .timeshare.yml is meant to be committed to git and world-readable; it never contains a credential
}

// runInit is the shared execution core for a fully-specified wizardState:
// create the vault, move/collect items, write .timeshare.yml. Both the
// --non-interactive path and the completed interactive wizard funnel
// through this — it doesn't know or care which one produced s.
func runInit(cwd string, s *wizardState) error {
	if err := s.validateComplete(); err != nil {
		return err
	}
	parsedTTL, err := time.ParseDuration(s.TTL)
	if err != nil {
		return fmt.Errorf("invalid --ttl: %w", err)
	}

	cfgPath := filepath.Join(cwd, ".timeshare.yml")
	if _, statErr := os.Stat(cfgPath); statErr == nil && !s.Force {
		return fmt.Errorf("%s already exists (pass --force to overwrite)", cfgPath)
	}

	fmt.Printf("Creating dedicated vault %q...\n", s.Vault)
	vaultID, err := onepassword.CreateVault(s.Vault)
	if err != nil {
		return fmt.Errorf("creating vault: %w", err)
	}

	items := append([]string{}, s.Items...)
	if s.MoveFrom != "" {
		existing, err := onepassword.ListItems(s.MoveFrom)
		if err != nil {
			return fmt.Errorf("listing items in %s: %w", s.MoveFrom, err)
		}
		for _, it := range existing {
			fmt.Printf("Moving %q into %q...\n", it.Title, s.Vault)
			if err := onepassword.MoveItem(it.ID, s.MoveFrom, s.Vault); err != nil {
				return fmt.Errorf("moving item %q failed (already moved: %v): %w", it.Title, items, err)
			}
			items = append(items, it.Title)
		}
	}

	for _, spec := range s.MoveItems {
		// Split on the rightmost "/" rather than the first, so a source
		// vault name that itself contains "/" still parses its item ref
		// correctly in the vault/item form. A bare vault name for the
		// interactive picker must not contain "/" at all — see the
		// flag's help text.
		var sourceVault, ref string
		var hasRef bool
		if idx := strings.LastIndex(spec, "/"); idx >= 0 {
			sourceVault, ref, hasRef = spec[:idx], spec[idx+1:], true
		} else {
			sourceVault = spec
		}

		var toMove []onepassword.Item
		if hasRef {
			item, err := onepassword.GetItem(sourceVault, ref)
			if err != nil {
				printSuggestions(sourceVault, ref)
				return fmt.Errorf("resolving %q in vault %q: %w", ref, sourceVault, err)
			}
			toMove = []onepassword.Item{item}
		} else {
			sourceItems, err := onepassword.ListItems(sourceVault)
			if err != nil {
				return fmt.Errorf("listing items in %s: %w", sourceVault, err)
			}
			picked, err := pickItems(sourceVault, sourceItems)
			if err != nil {
				return fmt.Errorf("picking items from %s: %w", sourceVault, err)
			}
			toMove = picked
		}

		for _, item := range toMove {
			fmt.Printf("Moving %q into %q...\n", item.Title, s.Vault)
			if err := onepassword.MoveItem(item.ID, sourceVault, s.Vault); err != nil {
				return fmt.Errorf("moving item %q failed (already moved: %v): %w", item.Title, items, err)
			}
			items = append(items, item.Title)
		}
	}

	cfg := config.Config{
		Vault: s.Vault,
		Mode:  config.Mode(s.Mode),
		TTL:   parsedTTL,
		Items: items,
	}

	if s.Mode == string(config.ModeServiceAccount) {
		fmt.Println("Create a service account:")
		fmt.Printf("  op service-account create %s --vault=%s:read_items\n", s.Vault+"-timeshare", vaultID)
		fmt.Println("Then store the printed token in your OS keychain:")
		fmt.Printf("  <paste token> | timeshare token store %s\n", s.Vault)
	}

	if err := writeTimeshareConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("writing .timeshare.yml: %w", err)
	}

	fmt.Printf("Wrote %s\n", cfgPath)
	return nil
}

func newInitCmd() *cobra.Command {
	var vaultName string
	var mode string
	var ttl string
	var moveFrom string
	var explicitItems []string
	var moveItems []string
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a dedicated vault and .timeshare.yml for this repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			s := newWizardState(cmd, vaultName, mode, ttl, moveFrom, explicitItems, moveItems, force)
			return runInit(cwd, s)
		},
	}

	cmd.Flags().StringVar(&vaultName, "vault", "", "name for the new dedicated vault")
	cmd.Flags().StringVar(&mode, "mode", string(config.ModeBiometric), "service-account or biometric")
	cmd.Flags().StringVar(&ttl, "ttl", "4h", "default cache TTL for this project")
	cmd.Flags().StringVar(&moveFrom, "move-from", "", "existing vault to move current items out of (optional)")
	cmd.Flags().StringArrayVar(&explicitItems, "item", nil, "item name to include in .timeshare.yml (repeatable); must already exist in --vault")
	cmd.Flags().StringArrayVar(&moveItems, "move-item", nil, "move one item from an existing vault: <source-vault>/<item-name-or-id>, or just <source-vault> (no slash) for an interactive picker (repeatable)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing .timeshare.yml")
	return cmd
}
```

Note: `RunE`'s error-return behavior changes slightly from today — the original code returned the raw `--vault is required` / `--ttl` parse errors directly from `RunE` before any side effect; `runInit` now returns the same errors via `s.validateComplete()`/`time.ParseDuration` at the top of the function, before any vault creation, so the observable behavior (error message, no side effects) is identical. This is exactly why Step 1/2 of this task are "confirm the baseline," not new tests — the contract doesn't change, only where the code lives.

- [ ] **Step 4: Run tests to verify they still pass**

Run: `go test ./internal/cli/...`
Expected: PASS, same test count as the Step 2 baseline

- [ ] **Step 5: Commit**

```bash
git add internal/cli/init.go
git commit -m "refactor: extract runInit as the shared wizardState-driven execution core"
```

---

## Task 5: Interactive wizard steps (vault name, auth mode, TTL)

**Files:**
- Create: `internal/cli/wizard.go`

**Interfaces:**
- Consumes: `wizardState`, `wizardStep`/`renderStepBlock` (Task 3), `defaultVaultName` (Task 1), `huh.NewInput`, `huh.NewSelect`
- Produces: `runWizard(cwd string, seeded *wizardState) (*wizardState, error)`, `isHelpRequest(input string) bool`

This task covers the vault-name, auth-mode, and TTL steps (blank or pre-seeded). The items step is Task 6.

- [ ] **Step 1: Write the failing test for the one pure piece of this file**

```go
// internal/cli/wizard_help_test.go
package cli

import "testing"

func TestIsHelpRequest(t *testing.T) {
	cases := map[string]bool{
		"?":  true,
		" ? ": true,
		"":   false,
		"4h": false,
	}
	for input, want := range cases {
		if got := isHelpRequest(input); got != want {
			t.Errorf("isHelpRequest(%q) = %v, want %v", input, got, want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cli/... -run TestIsHelpRequest`
Expected: FAIL with "undefined: isHelpRequest"

- [ ] **Step 3: Implement**

```go
// internal/cli/wizard.go
package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/newtosh/timeshare/internal/config"
)

const (
	stepVault = iota
	stepMode
	stepItems
	stepTTL
	stepCount
)

var wizardStepLabels = [stepCount]string{
	stepVault: "Vault name",
	stepMode:  "Auth mode",
	stepItems: "Items",
	stepTTL:   "TTL",
}

var wizardStepHelp = [stepCount]string{
	stepVault: "The name of a new, dedicated 1Password vault timeshare will create for this project. Pick something specific to this repo — it shouldn't be shared with unrelated projects.",
	stepMode:  "Biometric: shells out to `op read`, same Touch ID/Windows Hello prompt you already get, cached for the TTL. Service account: headless, token-based, no prompts at all, but requires a manual token-store step after init (see the printed instructions).",
	stepItems: "Which 1Password items should this project's allow-list include. You can move items from an existing vault, or pick from a list interactively.",
	stepTTL:   "How long a resolved secret stays cached before the next read re-checks 1Password. Longer means fewer prompts but a longer window before a rotated/revoked secret takes effect.",
}

// isHelpRequest reports whether a raw prompt input was a bare "?" (possibly
// surrounded by whitespace), the wizard's help-toggle signal.
func isHelpRequest(input string) bool {
	return strings.TrimSpace(input) == "?"
}

// promptWithHelp runs prompt in a loop: if the result is a bare "?", it
// prints that step's help text and re-prompts instead of returning.
func promptWithHelp(step int, prompt func() (string, error)) (string, error) {
	for {
		val, err := prompt()
		if err != nil {
			return "", err
		}
		if !isHelpRequest(val) {
			return val, nil
		}
		fmt.Println(wizardStepHelp[step])
	}
}

// runWizard walks the vault/mode/items/ttl steps, skipping any step whose
// value was already seeded from a flag (seeded.set[...] true), and returns
// the completed state. It does not call runInit — the caller (newInitCmd's
// RunE, Task 7) does that once the wizard returns.
func runWizard(cwd string, seeded *wizardState) (*wizardState, error) {
	s := &wizardState{
		Vault: seeded.Vault, Mode: seeded.Mode, TTL: seeded.TTL,
		MoveFrom: seeded.MoveFrom, Items: seeded.Items, MoveItems: seeded.MoveItems,
		Force: seeded.Force, set: seeded.set,
	}

	steps := func() []wizardStep {
		return []wizardStep{
			{Label: wizardStepLabels[stepVault], Value: s.Vault, Done: s.Vault != ""},
			{Label: wizardStepLabels[stepMode], Value: s.Mode, Done: s.set["mode"] || s.Mode != ""},
			{Label: wizardStepLabels[stepItems], Value: itemsSummary(s), Done: len(s.Items) > 0 || len(s.MoveItems) > 0 || s.MoveFrom != ""},
			{Label: wizardStepLabels[stepTTL], Value: s.TTL, Done: s.set["ttl"]},
		}
	}

	if !s.set["vault"] {
		fmt.Print(renderStepBlock(steps(), stepVault, "Vault name:") + "\n")
		val, err := promptWithHelp(stepVault, func() (string, error) {
			v := s.Vault
			if v == "" {
				v = defaultVaultName(cwd)
			}
			err := huh.NewInput().Title("Vault name").Value(&v).Run()
			return v, err
		})
		if err != nil {
			return nil, err
		}
		s.Vault = val
	}

	if !s.set["mode"] {
		fmt.Print(renderStepBlock(steps(), stepMode, "Auth mode:") + "\n")
		mode := s.Mode
		if mode == "" {
			mode = string(config.ModeBiometric)
		}
		if err := huh.NewSelect[string]().
			Title("Auth mode").
			Options(
				huh.NewOption("Biometric (recommended)", string(config.ModeBiometric)),
				huh.NewOption("Service account", string(config.ModeServiceAccount)),
			).
			Value(&mode).
			Run(); err != nil {
			return nil, err
		}
		s.Mode = mode
	}

	// Items step: Task 6.

	if !s.set["ttl"] {
		fmt.Print(renderStepBlock(steps(), stepTTL, "Default cache TTL:") + "\n")
		val, err := promptWithHelp(stepTTL, func() (string, error) {
			ttl := s.TTL
			if ttl == "" {
				ttl = "4h"
			}
			err := huh.NewInput().Title("Default cache TTL").Value(&ttl).Run()
			return ttl, err
		})
		if err != nil {
			return nil, err
		}
		s.TTL = val
	}

	return s, nil
}

func itemsSummary(s *wizardState) string {
	n := len(s.Items) + len(s.MoveItems)
	if s.MoveFrom != "" {
		return "from " + s.MoveFrom
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d selected", n)
}
```

Note: the Items step is intentionally left as a comment placeholder (`// Items step: Task 6.`) inside `runWizard` — Task 6 fills it in. This is not a plan placeholder violation (the plan's own text has no TODOs), it's the actual shape of an in-progress function across two tasks in the same file; Task 6's diff replaces that comment with real code.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run TestIsHelpRequest`
Expected: PASS (1 test)

Run: `go build ./...`
Expected: succeeds (the comment placeholder for the items step is valid Go — it's a no-op comment, not a compile error, since Task 6 adds real statements there without needing to close any open block)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/wizard.go internal/cli/wizard_help_test.go
git commit -m "feat: add wizard steps for vault name, auth mode, and TTL"
```

---

## Task 6: Wizard items step

**Files:**
- Modify: `internal/cli/wizard.go`

**Interfaces:**
- Consumes: `pickItems` (existing, `internal/cli/pick.go`), `onepassword.ListItems`
- Produces: fills in the `runWizard` items step

- [ ] **Step 1: Replace the placeholder comment with the real step**

```go
// internal/cli/wizard.go — replace the line `// Items step: Task 6.` with:

	if !s.set["item"] && !s.set["move-from"] && !s.set["move-item"] {
		fmt.Print(renderStepBlock(steps(), stepItems, "Items:\n\nMove items from an existing vault, or add them\nyourself later and edit .timeshare.yml by hand.") + "\n")

		var sourceVault string
		if err := huh.NewInput().
			Title("Existing vault to pick items from (leave blank to skip and add items later)").
			Value(&sourceVault).
			Run(); err != nil {
			return nil, err
		}

		if sourceVault != "" {
			sourceItems, err := onepassword.ListItems(sourceVault)
			if err != nil {
				return nil, fmt.Errorf("listing items in %s: %w", sourceVault, err)
			}
			picked, err := pickItems(sourceVault, sourceItems)
			if err != nil {
				return nil, fmt.Errorf("picking items from %s: %w", sourceVault, err)
			}
			s.MoveItems = make([]string, len(picked))
			for i, item := range picked {
				s.MoveItems[i] = sourceVault + "/" + item.ID
			}
		}
	}
```

This produces `MoveItems` entries in the same `<source-vault>/<item-id>` form `runInit` (Task 4) already parses via `strings.LastIndex(spec, "/")` — the wizard doesn't need its own move-item execution path, it just constructs the same input shape the existing flag-driven code already knows how to consume.

Add the `onepassword` import to `wizard.go`'s import block (it already imports `huh`, `config`, `fmt`, `strings` from Task 5 — add `"github.com/newtosh/timeshare/internal/onepassword"`).

- [ ] **Step 2: Build to confirm it compiles**

Run: `go build ./...`
Expected: succeeds

- [ ] **Step 3: Run the full non-interactive test suite (regression check — this task doesn't add new unit tests, per the Global Constraints note on interactive code)**

Run: `go test ./internal/cli/...`
Expected: PASS, same tests as before this task (no new failures introduced)

- [ ] **Step 4: Commit**

```bash
git add internal/cli/wizard.go
git commit -m "feat: add wizard items step, reusing the existing dual-list picker"
```

---

## Task 7: Existing-config summary and menu

**Files:**
- Create: `internal/cli/wizard_summary.go`

**Interfaces:**
- Consumes: `loadExistingConfig` (Task 2), `runWizard` (Tasks 5-6), `runInit` (Task 4), `huh.NewSelect`
- Produces: `runExistingConfigMenu(cwd string, cfgPath string, cfg config.Config) error`

- [ ] **Step 1: Implement**

No new unit test in this task — it's pure interactive glue over already-tested pieces (`loadExistingConfig`, `runWizard`, `runInit`), matching the Global Constraints note.

```go
// internal/cli/wizard_summary.go
package cli

import (
	"fmt"
	"time"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"

	"github.com/charmbracelet/huh"
)

const (
	menuAddItems  = "add-items"
	menuChangeTTL = "change-ttl"
	menuStartOver = "start-over"
)

// runExistingConfigMenu handles `timeshare init` (wizard mode) when
// .timeshare.yml already exists: show a summary, then act on one targeted
// choice. Never mutates the file until a choice is confirmed.
func runExistingConfigMenu(cwd, cfgPath string, cfg config.Config) error {
	fmt.Printf("Existing config at %s:\n", cfgPath)
	fmt.Printf("  vault: %s\n  mode:  %s\n  ttl:   %s\n  items: %d\n\n", cfg.Vault, cfg.Mode, cfg.TTL, len(cfg.Items))

	var choice string
	if err := huh.NewSelect[string]().
		Title("What would you like to do?").
		Options(
			huh.NewOption("Add items", menuAddItems),
			huh.NewOption("Change TTL", menuChangeTTL),
			huh.NewOption("Start over", menuStartOver),
		).
		Value(&choice).
		Run(); err != nil {
		return err
	}

	switch choice {
	case menuAddItems:
		var sourceVault string
		if err := huh.NewInput().Title("Existing vault to pick items from").Value(&sourceVault).Run(); err != nil {
			return err
		}
		sourceItems, err := onepassword.ListItems(sourceVault)
		if err != nil {
			return fmt.Errorf("listing items in %s: %w", sourceVault, err)
		}
		picked, err := pickItems(sourceVault, sourceItems)
		if err != nil {
			return fmt.Errorf("picking items from %s: %w", sourceVault, err)
		}
		items := append([]string{}, cfg.Items...)
		for _, item := range picked {
			fmt.Printf("Moving %q into %q...\n", item.Title, cfg.Vault)
			if err := onepassword.MoveItem(item.ID, sourceVault, cfg.Vault); err != nil {
				return fmt.Errorf("moving item %q failed (already moved: %v): %w", item.Title, items, err)
			}
			items = append(items, item.Title)
		}
		cfg.Items = items
		return writeTimeshareConfig(cfgPath, cfg)

	case menuChangeTTL:
		ttl := cfg.TTL.String()
		if err := huh.NewInput().Title("New default cache TTL").Value(&ttl).Run(); err != nil {
			return err
		}
		parsed, err := time.ParseDuration(ttl)
		if err != nil {
			return fmt.Errorf("invalid TTL: %w", err)
		}
		// Rewriting the file directly, not going through runInit: the
		// vault already exists, and runInit unconditionally calls
		// onepassword.CreateVault, which would be wrong here — this is a
		// pure config edit, no 1Password mutation at all.
		cfg.TTL = parsed
		return writeTimeshareConfig(cfgPath, cfg)

	case menuStartOver:
		seeded := &wizardState{set: make(map[string]bool)}
		completed, err := runWizard(cwd, seeded)
		if err != nil {
			return err
		}
		completed.Force = true
		return runInit(cwd, completed)
	}

	return nil
}
```

- [ ] **Step 2: Build to confirm it compiles**

Run: `go build ./...`
Expected: succeeds

- [ ] **Step 3: Commit**

```bash
git add internal/cli/wizard_summary.go
git commit -m "feat: add existing-config summary and menu (add items / change TTL / start over)"
```

---

## Task 8: Wire the new entry-mode dispatch into `newInitCmd`

**Files:**
- Modify: `internal/cli/init.go`
- Modify: `internal/cli/init_test.go`

**Interfaces:**
- Consumes: `newWizardState`, `runInit` (Task 4), `loadExistingConfig` (Task 2), `runExistingConfigMenu` (Task 7), `runWizard` (Tasks 5-6)
- Produces: `newInitCmd()`'s final `RunE`, new `--non-interactive`/`-n` flag, short flags on the existing ones

- [ ] **Step 1: Write the failing test for the one new piece of dispatch logic worth a unit test — that short flags actually register**

```go
// internal/cli/init_test.go — append
func TestInitCmdHasShortFlags(t *testing.T) {
	cmd := newInitCmd()
	for long, short := range map[string]string{
		"vault": "v", "mode": "m", "ttl": "t", "item": "i", "force": "f", "non-interactive": "n",
	} {
		f := cmd.Flags().ShorthandLookup(short)
		if f == nil || f.Name != long {
			t.Errorf("expected -%s to be the shorthand for --%s, got %+v", short, long, f)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cli/... -run TestInitCmdHasShortFlags`
Expected: FAIL (either a panic from `ShorthandLookup` on an unset shorthand, or a nil `f`, depending on Cobra's behavior for an unregistered short flag — either way, not passing)

- [ ] **Step 3: Implement — replace `newInitCmd`'s flag registration and `RunE`**

```go
// internal/cli/init.go — replace newInitCmd's body from the cmd.Flags() calls onward,
// and replace RunE with:

		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			s := newWizardState(cmd, vaultName, mode, ttl, moveFrom, explicitItems, moveItems, force)

			if nonInteractive {
				return runInit(cwd, s)
			}

			cfgPath := filepath.Join(cwd, ".timeshare.yml")
			existingCfg, exists, loadErr := loadExistingConfig(cfgPath)
			if loadErr != nil {
				return fmt.Errorf("reading existing %s: %w", cfgPath, loadErr)
			}
			if exists {
				return runExistingConfigMenu(cwd, cfgPath, existingCfg)
			}

			completed, err := runWizard(cwd, s)
			if err != nil {
				return err
			}
			return runInit(cwd, completed)
		},
	}

	cmd.Flags().StringVarP(&vaultName, "vault", "v", "", "name for the new dedicated vault")
	cmd.Flags().StringVarP(&mode, "mode", "m", string(config.ModeBiometric), "service-account or biometric")
	cmd.Flags().StringVarP(&ttl, "ttl", "t", "4h", "default cache TTL for this project")
	cmd.Flags().StringVar(&moveFrom, "move-from", "", "existing vault to move current items out of (optional)")
	cmd.Flags().StringArrayVarP(&explicitItems, "item", "i", nil, "item name to include in .timeshare.yml (repeatable); must already exist in --vault")
	cmd.Flags().StringArrayVar(&moveItems, "move-item", nil, "move one item from an existing vault: <source-vault>/<item-name-or-id>, or just <source-vault> (no slash) for an interactive picker (repeatable)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing .timeshare.yml")
	cmd.Flags().BoolVarP(&nonInteractive, "non-interactive", "n", false, "never prompt; validate flags and fail fast on anything incomplete (for scripts/CI)")
	return cmd
}
```

Also add `var nonInteractive bool` alongside the other `var` declarations at the top of `newInitCmd`.

The full corrected `newInitCmd` (for clarity — this replaces the version from Task 4 in its entirety):

```go
func newInitCmd() *cobra.Command {
	var vaultName string
	var mode string
	var ttl string
	var moveFrom string
	var explicitItems []string
	var moveItems []string
	var force bool
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a dedicated vault and .timeshare.yml for this repo — guided wizard if no flags are given",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			s := newWizardState(cmd, vaultName, mode, ttl, moveFrom, explicitItems, moveItems, force)

			if nonInteractive {
				return runInit(cwd, s)
			}

			cfgPath := filepath.Join(cwd, ".timeshare.yml")
			existingCfg, exists, loadErr := loadExistingConfig(cfgPath)
			if loadErr != nil {
				return fmt.Errorf("reading existing %s: %w", cfgPath, loadErr)
			}
			if exists {
				return runExistingConfigMenu(cwd, cfgPath, existingCfg)
			}

			completed, err := runWizard(cwd, s)
			if err != nil {
				return err
			}
			return runInit(cwd, completed)
		},
	}

	cmd.Flags().StringVarP(&vaultName, "vault", "v", "", "name for the new dedicated vault")
	cmd.Flags().StringVarP(&mode, "mode", "m", string(config.ModeBiometric), "service-account or biometric")
	cmd.Flags().StringVarP(&ttl, "ttl", "t", "4h", "default cache TTL for this project")
	cmd.Flags().StringVar(&moveFrom, "move-from", "", "existing vault to move current items out of (optional)")
	cmd.Flags().StringArrayVarP(&explicitItems, "item", "i", nil, "item name to include in .timeshare.yml (repeatable); must already exist in --vault")
	cmd.Flags().StringArrayVar(&moveItems, "move-item", nil, "move one item from an existing vault: <source-vault>/<item-name-or-id>, or just <source-vault> (no slash) for an interactive picker (repeatable)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing .timeshare.yml")
	cmd.Flags().BoolVarP(&nonInteractive, "non-interactive", "n", false, "never prompt; validate flags and fail fast on anything incomplete (for scripts/CI)")
	return cmd
}
```

Note: since bare `timeshare init` (zero flags, `nonInteractive` false) now drops into `runWizard` instead of erroring, the pre-existing `TestWriteTimeshareConfig`-style tests that call `writeTimeshareConfig` directly are unaffected (they don't go through `RunE`), but any test that previously relied on running the full `init` command with zero flags and expecting an immediate `--vault is required` error would now hang waiting on interactive input instead. Confirm no such test exists before proceeding (Step 4 covers this).

- [ ] **Step 4: Check for and fix any test relying on the old zero-flag error behavior**

Run: `grep -rn 'newInitCmd()' internal/cli/*_test.go`

If any test constructs `newInitCmd()` and calls `.Execute()` or invokes `RunE` directly with zero flags expecting an error return (rather than just checking flag registration, as this task's own new test does), it would now block on stdin. Based on the current test suite (`internal/cli/init_test.go`'s `TestWriteTimeshareConfig` calls `writeTimeshareConfig` directly, not through `RunE`), no such test exists — but re-run this grep after the refactor to confirm before moving on, and if one is found, guard it with `--non-interactive` in its invocation rather than deleting coverage.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run TestInitCmdHasShortFlags`
Expected: PASS (1 test)

Run: `go build ./... && go test ./internal/cli/...`
Expected: build succeeds, full package test suite passes

- [ ] **Step 6: Commit**

```bash
git add internal/cli/init.go internal/cli/init_test.go
git commit -m "feat: wire guided-wizard entry-mode dispatch into timeshare init"
```

---

## Self-Review Notes

**Spec coverage:**
- Entry mode 1 (`--non-interactive`) → Task 8, unchanged validation logic → Task 1/4
- Entry mode 2 (existing config → summary+menu) → Tasks 2, 7, 8
- Entry mode 3 (blank wizard) → Tasks 1 (defaultVaultName), 5, 6, 8
- Entry mode 4 (pre-seeded wizard) → Task 1 (`set` map via `Changed()`), Tasks 5/6 (`if !s.set[...]` skip checks)
- Step-tracker two-column layout, completed-step values in color, `?` help hint line → Tasks 3, 5
- Items step reuses `pickItems` → Task 6
- Short flags `-v/-m/-t/-i/-f/-n`, `--move-from`/`--move-item` long-form only → Task 8
- Shared execution core (no duplicated vault-creation/move logic between paths) → Task 4
- `?` help is local/static, never network → Task 5 (`wizardStepHelp` is a compile-time string table)
- Existing-config menu never mutates until a choice is confirmed → Task 7 (`huh.NewSelect.Run()` blocks until confirmed; no file write happens before that call returns)

**Placeholder scan:** the one `// Items step: Task 6.` comment is explicitly called out and justified (Task 5's own text explains why it's not a plan violation, and Task 6 immediately replaces it) — not a TBD/TODO left unresolved by the plan itself.

**Type consistency:** `wizardState` fields (`Vault, Mode, TTL, MoveFrom string; Items, MoveItems []string; Force bool; set map[string]bool`) are used identically across Tasks 1, 4, 5, 6, 7, 8 — `runInit(cwd string, s *wizardState) error`, `runWizard(cwd string, seeded *wizardState) (*wizardState, error)`, `newWizardState(cmd *cobra.Command, vault, mode, ttl, moveFrom string, items, moveItems []string, force bool) *wizardState` all match their call sites. `wizardStep{Label, Value string; Done bool}` and `renderStepBlock(steps []wizardStep, activeIdx int, activeContent string) string` (Task 3) match their usage in Task 5's `runWizard`.
