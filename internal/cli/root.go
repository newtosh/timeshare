package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/projectid"
	"github.com/newtosh/timeshare/internal/trace"
	"github.com/newtosh/timeshare/internal/version"

	"github.com/spf13/cobra"
)

// verbose is set by --verbose. TIMESHARE_DEBUG is checked as well.
var verbose bool

// LoadProjectContext finds the git root above cwd, loads its .timeshare.yml,
// and derives the project ID used as the daemon's cache-key prefix. Every
// CLI command that talks to the daemon starts here (spec: no daemon spawn,
// no guessing, when config is absent).
func LoadProjectContext(cwd string) (config.Config, string, error) {
	root, err := projectid.FindGitRoot(cwd)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("not inside a git repository: %w", err)
	}
	common, err := projectid.CommonDir(root)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("not inside a git repository: %w", err)
	}

	// Prefer a .timeshare.yml in this checkout (a worktree usually has
	// the committed file). Fall back to the main checkout so a linked
	// worktree still finds the repo's config.
	cfgPath := filepath.Join(root, ".timeshare.yml")
	if _, err := os.Stat(cfgPath); err != nil {
		if main, ok := projectid.MainCheckout(common); ok && main != root {
			alt := filepath.Join(main, ".timeshare.yml")
			if _, statErr := os.Stat(alt); statErr == nil {
				cfgPath = alt
			}
		}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("no valid .timeshare.yml found (run `timeshare init`): %w", err)
	}

	return cfg, projectid.Derive(common), nil
}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "timeshare",
		Short:         "Repo-scoped, TTL-cached secret bridge for 1Password",
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "trace each op call and each proxied sign (or set TIMESHARE_DEBUG=1)")
	cobra.OnInitialize(func() {
		if verbose || trace.EnvEnabled() {
			trace.SetEnabled(true)
		}
	})
	root.AddCommand(newReadCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newLockCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newTokenCmd())
	return root
}
