// internal/cli/doctor.go (stub, replaced in Task 12)
package cli

import "github.com/spf13/cobra"

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Diagnose timeshare and 1Password CLI health", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
