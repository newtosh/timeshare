// internal/cli/lock.go (stub, replaced in Task 12)
package cli

import "github.com/spf13/cobra"

func newLockCmd() *cobra.Command {
	return &cobra.Command{Use: "lock", Short: "Evict cached secrets for the current project", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
