package cli

import (
	"fmt"
	"os"

	"github.com/newtosh/timeshare/internal/client"
	"github.com/newtosh/timeshare/internal/daemon"

	"github.com/spf13/cobra"
)

func newReadCmd() *cobra.Command {
	var ttlOverride string

	cmd := &cobra.Command{
		Use:   "read <secret-name>",
		Short: "Resolve one secret and print it to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretName := args[0]

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}
			if !cfg.Allows(secretName) {
				return fmt.Errorf("%q is not in this project's .timeshare.yml items list", secretName)
			}

			ttl := cfg.TTL
			if ttlOverride != "" {
				parsed, err := parseDuration(ttlOverride)
				if err != nil {
					return fmt.Errorf("invalid --ttl: %w", err)
				}
				ttl = parsed
			}

			c := &client.Client{
				SocketPath:   client.DefaultSocketPath(),
				DaemonBinary: daemonBinaryPath(),
			}
			value, err := c.Read(cmd.Context(), daemon.Request{
				ProjectID:    projectID,
				SecretName:   secretName,
				Vault:        cfg.Vault,
				Mode:         cfg.Mode,
				TTL:          ttl,
				Field:        cfg.FieldFor(secretName),
				AllowedItems: cfg.ItemNames(),
			})
			if err != nil {
				return err
			}

			fmt.Println(value)
			return nil
		},
	}

	cmd.Flags().StringVar(&ttlOverride, "ttl", "", "override this project's default TTL for this invocation (e.g. 30m)")
	return cmd
}
