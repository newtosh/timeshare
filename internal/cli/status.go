package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"timeshare/internal/client"
	"timeshare/internal/daemon"
)

var okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Confirm the daemon is reachable for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}

			c := &client.Client{SocketPath: client.DefaultSocketPath(), DaemonBinary: daemonBinaryPath()}
			_, err = c.Read(cmd.Context(), daemon.Request{ProjectID: projectID, AllowedItems: cfg.Items, Op: daemon.OpStatus})
			if err != nil {
				return fmt.Errorf("daemon unreachable: %w", err)
			}

			fmt.Println(okStyle.Render("✓") + fmt.Sprintf(" daemon reachable for vault %q (%d items configured)", cfg.Vault, len(cfg.Items)))
			return nil
		},
	}
}
