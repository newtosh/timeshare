package cli

import (
	"os/exec"
	"time"
)

func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// daemonBinaryPath locates the timesharedd binary installed alongside
// timeshare (same directory), falling back to PATH lookup.
func daemonBinaryPath() string {
	if p, err := exec.LookPath("timesharedd"); err == nil {
		return p
	}
	return "timesharedd"
}
