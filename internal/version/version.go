// Package version holds build-time identity for the timeshare binaries.
// Values are overwritten via -ldflags at release / `make build` time;
// local `go build` / `go install` without ldflags keeps the "dev" defaults.
package version

import (
	"fmt"
	"runtime/debug"
)

// These are set by -ldflags, e.g.:
//
//	-X github.com/newtosh/timeshare/internal/version.Version=v0.1.3
//	-X github.com/newtosh/timeshare/internal/version.Commit=abc123
//	-X github.com/newtosh/timeshare/internal/version.Date=2026-01-01T00:00:00Z
//
// `make build`, GoReleaser, and install.sh pass them. A plain
// `go build` keeps the defaults below, then String falls back to the
// module version and VCS settings the toolchain stamps into the binary.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a single-line identity suitable for --version output.
func String() string {
	version, commit, date := Version, Commit, Date
	if info, ok := debug.ReadBuildInfo(); ok {
		if (version == "dev" || version == "") && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		var rev, built string
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.time":
				built = s.Value
			}
		}
		if (commit == "none" || commit == "") && rev != "" {
			if len(rev) > 12 {
				rev = rev[:12]
			}
			commit = rev
		}
		if (date == "unknown" || date == "") && built != "" {
			date = built
		}
	}
	return fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
}
