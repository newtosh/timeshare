package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/newtosh/timeshare/internal/tokenstore"

	"github.com/spf13/cobra"
)

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage 1Password service-account tokens in the OS keychain",
	}
	cmd.AddCommand(newTokenStoreCmd())
	cmd.AddCommand(newTokenDeleteCmd())
	return cmd
}

func newTokenStoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "store <vault>",
		Short: "Read a token from stdin and store it in the OS keychain for <vault>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Paste token, then press Enter:")
			if err := runTokenStore(args[0], cmd.InOrStdin()); err != nil {
				return err
			}
			fmt.Println("Token stored.")
			return nil
		},
	}
}

func newTokenDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <vault>",
		Short: "Remove <vault>'s stored token from the OS keychain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runTokenDelete(args[0]); err != nil {
				return err
			}
			fmt.Println("Token deleted.")
			return nil
		},
	}
}

func runTokenStore(vault string, in io.Reader) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	var token string
	if scanner.Scan() {
		token = strings.TrimSpace(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading token: %w", err)
	}
	if token == "" {
		return fmt.Errorf("empty token")
	}
	return tokenstore.Store(vault, token)
}

func runTokenDelete(vault string) error {
	return tokenstore.Delete(vault)
}
