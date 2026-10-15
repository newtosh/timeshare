package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/newtosh/timeshare/internal/client"
	"github.com/newtosh/timeshare/internal/daemon"
	"github.com/newtosh/timeshare/internal/trace"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether a pass is active for the current project and when it expires",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}

			if !needsDaemon(cfg) {
				fmt.Println(okStyle.Render("✓") + " daemon not required")
				return nil
			}

			c := &client.Client{SocketPath: client.DefaultSocketPath(), DaemonBinary: daemonBinaryPath()}
			resp, err := c.Do(cmd.Context(), daemon.Request{
				Op:        daemon.OpStatus,
				ProjectID: projectID,
				Debug:     trace.Enabled(),
			})
			if err != nil {
				fmt.Println("daemon not running")
				return err
			}

			fmt.Println(okStyle.Render("✓") + fmt.Sprintf(" daemon reachable for vault %q (%d items, %d ssh keys configured)", cfg.Vault, len(cfg.Items), len(cfg.SSHKeys)))
			fmt.Println(formatPasses(resp.Passes, time.Now()))
			return nil
		},
	}
}

// formatPasses renders the daemon's active grants. An empty list is a
// reachable daemon with nothing cached yet.
func formatPasses(passes []daemon.Pass, now time.Time) string {
	if len(passes) == 0 {
		return "no active pass"
	}
	var b strings.Builder
	b.WriteString("active pass:")
	for _, p := range passes {
		remain := p.ExpiresAt.Sub(now).Round(time.Second)
		if remain < 0 {
			remain = 0
		}
		_, _ = fmt.Fprintf(&b, "\n  %s %s expires %s (in %s)", p.Kind, p.Name, p.ExpiresAt.Format(time.RFC3339), remain)
	}
	return b.String()
}
