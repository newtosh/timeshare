package cli

import (
	"context"
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

			c := &client.Client{SocketPath: client.DefaultSocketPath()}
			if err := checkDaemonReachable(cmd.Context(), c); err != nil {
				fmt.Println("daemon not running")
				return err
			}

			fmt.Println(okStyle.Render("✓") + fmt.Sprintf(" daemon reachable for vault %q (%d items configured)", cfg.Vault, len(cfg.Items)))
			return nil
		},
	}
}

// checkDaemonReachable is a diagnostic probe: it must never spawn the
// daemon it's reporting on, so it dials the raw socket (PingSocket)
// instead of going through client.Read's spawn-on-failure path.
func checkDaemonReachable(ctx context.Context, c *client.Client) error {
	conn, err := c.PingSocket(ctx)
	if err != nil {
		return fmt.Errorf("daemon unreachable at %s: %w", c.SocketPath, err)
	}
	_ = conn.Close()
	return nil
}
