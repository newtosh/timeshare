// internal/cli/status.go (stub, replaced in Task 12)
package cli

import "github.com/spf13/cobra"

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show cache TTLs for the current project", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
