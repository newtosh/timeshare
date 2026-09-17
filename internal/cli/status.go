package cli

import (
	"fmt"
	"os"

	"timeshare/internal/client"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
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
			cfg, _, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}

			// status is a diagnostic probe: it must never spawn the daemon
			// it's supposed to be reporting on, so it dials the raw socket
			// (PingSocket) instead of going through client.Read's
			// spawn-on-failure path.
			c := &client.Client{SocketPath: client.DefaultSocketPath()}
			conn, err := c.PingSocket(cmd.Context())
			if err != nil {
				fmt.Println("daemon not running")
				return nil
			}
			_ = conn.Close()

			fmt.Println(okStyle.Render("✓") + fmt.Sprintf(" daemon reachable for vault %q (%d items configured)", cfg.Vault, len(cfg.Items)))
			return nil
		},
	}
}
