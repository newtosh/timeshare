package cli

import (
	"fmt"
	"path/filepath"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/projectid"

	"github.com/spf13/cobra"
)

// LoadProjectContext finds the git root above cwd, loads its .timeshare.yml,
// and derives the project ID used as the daemon's cache-key prefix. Every
// CLI command that talks to the daemon starts here (spec: no daemon spawn,
// no guessing, when config is absent).
func LoadProjectContext(cwd string) (config.Config, string, error) {
	root, err := projectid.FindGitRoot(cwd)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("not inside a git repository: %w", err)
	}

	cfgPath := filepath.Join(root, ".timeshare.yml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("no valid .timeshare.yml found (run `timeshare init`): %w", err)
	}

	return cfg, projectid.Derive(root), nil
}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "timeshare",
		Short:         "Repo-scoped, TTL-cached secret bridge for 1Password",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newReadCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newLockCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newTokenCmd())
	return root
}
