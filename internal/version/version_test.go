package version_test

import (
	"strings"
	"testing"

	"github.com/newtosh/timeshare/internal/version"
)

func TestStringShape(t *testing.T) {
	s := version.String()
	if !strings.Contains(s, "(commit ") || !strings.Contains(s, ", built ") {
		t.Fatalf("String() = %q", s)
	}
}

func TestStringPrefersExplicitLdflags(t *testing.T) {
	origV, origC, origD := version.Version, version.Commit, version.Date
	t.Cleanup(func() {
		version.Version, version.Commit, version.Date = origV, origC, origD
	})
	version.Version = "v1.2.3"
	version.Commit = "abc123def456"
	version.Date = "2026-01-01T00:00:00Z"
	s := version.String()
	for _, want := range []string{"v1.2.3", "abc123def456", "2026-01-01T00:00:00Z"} {
		if !strings.Contains(s, want) {
			t.Fatalf("String() = %q, missing %q", s, want)
		}
	}
}
