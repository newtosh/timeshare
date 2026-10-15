package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/newtosh/timeshare/internal/client"
	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/daemon"
	"github.com/newtosh/timeshare/internal/onepassword"
	"github.com/newtosh/timeshare/internal/sshagent"
	"github.com/newtosh/timeshare/internal/trace"
	"github.com/newtosh/timeshare/internal/usermsg"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// Seams so doctor existence checks are unit-testable without shelling to op.
var doctorGetItem = onepassword.GetItem
var doctorGetSSHFingerprint = onepassword.GetItemFingerprint

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
			var cfg config.Config
			var projectID string
			configLoaded := false
			if !check(".timeshare.yml found", func() error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				loaded, id, err := LoadProjectContext(cwd)
				cfg = loaded
				projectID = id
				return err
			}) {
				failed++
			} else {
				configLoaded = true
			}

			label, spawn := daemonCheckPlan(configLoaded, cfg)
			if !check(label, func() error {
				if !spawn && label == "daemon not required" {
					return nil
				}
				c := &client.Client{SocketPath: client.DefaultSocketPath()}
				if spawn {
					c.DaemonBinary = daemonBinaryPath()
					if _, err := c.Do(cmd.Context(), daemon.Request{
						Op:        daemon.OpStatus,
						ProjectID: projectID,
						Debug:     trace.Enabled(),
					}); err != nil {
						return err
					}
					return nil
				}
				conn, err := c.PingSocket(cmd.Context())
				if err == nil {
					_ = conn.Close()
				}
				return err
			}) {
				failed++
			}

			if len(cfg.SSHKeys) > 0 {
				if !check("SSH agent reachable (needed for ssh_keys)", func() error {
					path := sshagent.UpstreamSocketPath()
					if path == "" {
						return fmt.Errorf("could not determine upstream SSH agent socket path")
					}
					conn, err := net.Dial("unix", path)
					if err != nil {
						return usermsg.Annotate(fmt.Errorf("connecting to upstream SSH agent at %s: %w", path, err))
					}
					return conn.Close()
				}) {
					failed++
				}
			}

			failed += reportRefs("configured item", "exist in vault", len(cfg.Items), verifyConfiguredItems(cfg))
			failed += reportRefs("configured ssh_key", "resolve", len(cfg.SSHKeys), verifyConfiguredSSHKeys(cfg.SSHKeys))

			if failed > 0 {
				return fmt.Errorf("%d check(s) failed", failed)
			}
			return nil
		},
	}
}

// reportRefs prints one pass line or per-error fail lines. n==0 skips.
func reportRefs(kind, passVerb string, n int, errs []error) int {
	if n == 0 {
		return 0
	}
	if len(errs) == 0 {
		fmt.Println(passStyle.Render(fmt.Sprintf("✓ %d %s(s) %s", n, kind, passVerb)))
		return 0
	}
	for _, err := range errs {
		fmt.Println(failStyle.Render("✗ "+kind) + ": " + err.Error())
	}
	return len(errs)
}

// needsDaemon reports whether this config uses the daemon. Item values
// and the SSH proxy both live there, so either list requires it. A
// config with neither (Load rejects that) does not.
func needsDaemon(cfg config.Config) bool {
	return len(cfg.Items) > 0 || len(cfg.SSHKeys) > 0
}

// daemonCheckPlan decides the doctor/status daemon line. When the loaded
// config needs the daemon, spawn is true and the caller auto-starts it.
// When a loaded config needs nothing, the check is "daemon not required"
// and passes without dialing.
func daemonCheckPlan(loaded bool, cfg config.Config) (label string, spawn bool) {
	if loaded && !needsDaemon(cfg) {
		return "daemon not required", false
	}
	return "daemon socket reachable", loaded && needsDaemon(cfg)
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

// verifyConfiguredItems returns one error per missing/unresolvable item
// in cfg.Vault. Metadata only — does not read secret values.
func verifyConfiguredItems(cfg config.Config) []error {
	var errs []error
	for _, item := range cfg.Items {
		if _, err := doctorGetItem(cfg.Vault, item.Name); err != nil {
			errs = append(errs, fmt.Errorf("item %q in vault %q: %w", item.Name, cfg.Vault, err))
		}
	}
	return errs
}

// verifyConfiguredSSHKeys returns one error per bad ssh_keys ref
// (<vault>/<item>). Uses the fingerprint field so non-SSH-Key items fail
// here instead of mid-run.
func verifyConfiguredSSHKeys(refs []string) []error {
	var errs []error
	for _, ref := range refs {
		idx := strings.LastIndex(ref, "/")
		if idx < 0 {
			errs = append(errs, fmt.Errorf("ssh_key %q must be in <vault>/<item> form", ref))
			continue
		}
		vault, item := ref[:idx], ref[idx+1:]
		if _, err := doctorGetSSHFingerprint(vault, item); err != nil {
			errs = append(errs, fmt.Errorf("ssh_key %q: %w", ref, usermsg.Annotate(err)))
		}
	}
	return errs
}
