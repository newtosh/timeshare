// Package trace is the debug log for op invocations and proxied SSH
// signs. It stays off unless the CLI was started with --verbose or
// TIMESHARE_DEBUG, so a normal run produces no extra output.
package trace

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

var (
	mu  sync.Mutex
	on  bool
	out io.Writer = os.Stderr
)

// SetEnabled turns tracing on or off for this process.
func SetEnabled(v bool) {
	mu.Lock()
	on = v
	mu.Unlock()
}

// Enabled reports whether tracing is on.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return on
}

// SetOutput redirects trace lines. Tests use a buffer; production uses stderr.
func SetOutput(w io.Writer) {
	mu.Lock()
	out = w
	mu.Unlock()
}

// EnvEnabled reports whether TIMESHARE_DEBUG requests tracing.
// Empty, 0, false, no, and off are off. Any other value is on.
func EnvEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TIMESHARE_DEBUG"))) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// Printf writes one trace line when tracing is enabled.
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if !on || out == nil {
		return
	}
	if _, err := fmt.Fprintf(out, "timeshare: "+format+"\n", args...); err != nil {
		return
	}
}
