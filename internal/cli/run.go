package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/newtosh/timeshare/internal/client"
	"github.com/newtosh/timeshare/internal/daemon"
	"github.com/newtosh/timeshare/internal/trace"

	"github.com/spf13/cobra"
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
			for _, item := range cfg.Items {
				value, err := c.Read(cmd.Context(), daemon.Request{
					ProjectID:    projectID,
					SecretName:   item.Name,
					Vault:        cfg.Vault,
					Mode:         cfg.Mode,
					TTL:          cfg.TTL,
					Field:        item.Field,
					AllowedItems: cfg.ItemNames(),
					Debug:        trace.Enabled(),
				})
				if err != nil {
					return fmt.Errorf("resolving %s: %w", item.Name, err)
				}
				// Replace, don't append. getenv (OpenSSH, most C programs)
				// returns the first match, so a second SSH_AUTH_SOCK or
				// secret variable would be ignored.
				env = replaceEnv(env, item.Name, value)
			}

			if len(cfg.SSHKeys) > 0 {
				trace.Printf("requesting ssh grant project=%s keys=%d ttl=%s", projectID, len(cfg.SSHKeys), cfg.TTL)
				resp, err := c.Do(cmd.Context(), daemon.Request{
					Op:        daemon.OpEnsureSSH,
					ProjectID: projectID,
					Vault:     cfg.Vault,
					Mode:      cfg.Mode,
					TTL:       cfg.TTL,
					SSHKeys:   cfg.SSHKeys,
					Debug:     trace.Enabled(),
				})
				if err != nil {
					return fmt.Errorf("starting SSH key proxy: %w", err)
				}
				if resp.SocketPath == "" {
					return fmt.Errorf("starting SSH key proxy: daemon returned no socket")
				}
				trace.Printf("ssh auth sock %s expires %s", resp.SocketPath, resp.ExpiresAt.Format(time.RFC3339))
				env = replaceEnv(env, "SSH_AUTH_SOCK", resp.SocketPath)
			}

			child := exec.Command(args[0], args[1:]...) //nolint:gosec // args come from the user's own command line, exactly like `env`/`op run`
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

// replaceEnv sets key=value, dropping any earlier assignment of key.
func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return append(out, prefix+value)
}
