package cli

import (
	"fmt"
	"os"

	"github.com/newtosh/timeshare/internal/client"
	"github.com/newtosh/timeshare/internal/daemon"
	"github.com/newtosh/timeshare/internal/trace"

	"github.com/spf13/cobra"
)

func newLockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Revoke this project's cached secrets and SSH grant",
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
			_, err = c.Do(cmd.Context(), daemon.Request{ProjectID: projectID, Op: daemon.OpLock, Debug: trace.Enabled()})
			if err != nil {
				return err
			}

			fmt.Println("Pass revoked for this project.")
			return nil
		},
	}
}
