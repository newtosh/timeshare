package version_test

import (
	"strings"
	"testing"

	"github.com/newtosh/timeshare/internal/version"
)

func TestStringIncludesDefaults(t *testing.T) {
	s := version.String()
	for _, want := range []string{version.Version, version.Commit, version.Date} {
		if !strings.Contains(s, want) {
			t.Fatalf("String() = %q, want it to contain %q", s, want)
		}
	}
}
