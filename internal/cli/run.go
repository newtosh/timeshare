package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"timeshare/internal/client"
	"timeshare/internal/daemon"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run -- <command> [args...]",
		Short: "Resolve every item in .timeshare.yml and exec a subprocess with them injected as env vars",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}

			c := &client.Client{
				SocketPath:   client.DefaultSocketPath(),
				DaemonBinary: daemonBinaryPath(),
			}

			env := os.Environ()
			for _, name := range cfg.Items {
				value, err := c.Read(cmd.Context(), daemon.Request{
					ProjectID:    projectID,
					SecretName:   name,
					Vault:        cfg.Vault,
					Mode:         cfg.Mode,
					TTL:          cfg.TTL,
					AllowedItems: cfg.Items,
				})
				if err != nil {
					return fmt.Errorf("resolving %s: %w", name, err)
				}
				env = append(env, name+"="+value)
			}

			child := exec.Command(args[0], args[1:]...)
			child.Env = env
			child.Stdin = os.Stdin
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			if err := child.Run(); err != nil {
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					os.Exit(ee.ExitCode())
				}
				return err
			}
			return nil
		},
	}
}
