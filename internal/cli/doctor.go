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
			failed := 0
			if !check("op CLI installed", func() error {
				_, err := exec.LookPath("op")
				return err
			}) {
				failed++
			}
			if !check("daemon socket reachable", func() error {
				c := &client.Client{SocketPath: client.DefaultSocketPath()}
				conn, err := c.PingSocket(cmd.Context())
				if err == nil {
					conn.Close()
				}
				return err
			}) {
				failed++
			}
			if !check(".timeshare.yml found", func() error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				_, _, err = LoadProjectContext(cwd)
				return err
			}) {
				failed++
			}
			if failed > 0 {
				return fmt.Errorf("%d check(s) failed", failed)
			}
			return nil
		},
	}
}

// check runs fn, prints a pass/fail line, and reports whether it passed.
func check(name string, fn func() error) bool {
	if err := fn(); err != nil {
		fmt.Println(failStyle.Render("✗ "+name) + ": " + err.Error())
		return false
	}
	fmt.Println(passStyle.Render("✓ " + name))
	return true
}
