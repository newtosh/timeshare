package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"timeshare/internal/client"
)

var (
	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose timeshare and 1Password CLI health",
		RunE: func(cmd *cobra.Command, args []string) error {
			check("op CLI installed", func() error {
				_, err := exec.LookPath("op")
				return err
			})
			check("daemon socket reachable", func() error {
				c := &client.Client{SocketPath: client.DefaultSocketPath()}
				conn, err := c.PingSocket(cmd.Context())
				if err == nil {
					conn.Close()
				}
				return err
			})
			check(".timeshare.yml found", func() error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				_, _, err = LoadProjectContext(cwd)
				return err
			})
			return nil
		},
	}
}

func check(name string, fn func() error) {
	if err := fn(); err != nil {
		fmt.Println(failStyle.Render("✗ "+name) + ": " + err.Error())
		return
	}
	fmt.Println(passStyle.Render("✓ " + name))
}
