// internal/cli/init.go (stub, replaced in Task 10)
package cli

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Scaffold a dedicated vault and .timeshare.yml for this repo", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
