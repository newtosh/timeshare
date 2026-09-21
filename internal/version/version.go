// Package version holds build-time identity for the timeshare binaries.
// Values are overwritten via -ldflags at release / `make build` time;
// local `go build` / `go install` without ldflags keeps the "dev" defaults.
package version

import "fmt"

// These are set by -ldflags, e.g.:
//
//	-X github.com/newtosh/timeshare/internal/version.Version=v0.1.3
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a single-line identity suitable for --version output.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}
