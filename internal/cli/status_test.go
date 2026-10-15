package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/daemon"
)

func TestFormatPassesEmpty(t *testing.T) {
	if got := formatPasses(nil, time.Now()); got != "no active pass" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatPassesShowsExpiry(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	exp := now.Add(4 * time.Hour)
	got := formatPasses([]daemon.Pass{
		{Kind: "item", Name: "DATABASE_URL", ExpiresAt: exp},
		{Kind: "ssh", Name: "Private/deploy-key", ExpiresAt: exp},
	}, now)
	for _, want := range []string{"active pass:", "item DATABASE_URL", "ssh Private/deploy-key", "expires " + exp.Format(time.RFC3339), "in 4h0m0s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestDaemonCheckPlan(t *testing.T) {
	label, spawn := daemonCheckPlan(true, config.Config{})
	if label != "daemon not required" || spawn {
		t.Fatalf("empty config: %q spawn=%v", label, spawn)
	}
	label, spawn = daemonCheckPlan(true, config.Config{SSHKeys: []string{"Private/k"}})
	if label != "daemon socket reachable" || !spawn {
		t.Fatalf("ssh keys: %q spawn=%v", label, spawn)
	}
	label, spawn = daemonCheckPlan(true, config.Config{Items: []config.Item{{Name: "X"}}})
	if label != "daemon socket reachable" || !spawn {
		t.Fatalf("items: %q spawn=%v", label, spawn)
	}
	label, spawn = daemonCheckPlan(false, config.Config{})
	if label != "daemon socket reachable" || spawn {
		t.Fatalf("missing config must not spawn: %q spawn=%v", label, spawn)
	}
}
