package cli

import (
	"fmt"
	"os"

	"timeshare/internal/client"
	"timeshare/internal/daemon"

	"github.com/spf13/cobra"
)

func newLockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Evict all cached secrets for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			_, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}

			c := &client.Client{SocketPath: client.DefaultSocketPath(), DaemonBinary: daemonBinaryPath()}
			_, err = c.Read(cmd.Context(), daemon.Request{ProjectID: projectID, Op: daemon.OpLock})
			if err != nil {
				return err
			}

			fmt.Println("Cache evicted for this project.")
			return nil
		},
	}
}
