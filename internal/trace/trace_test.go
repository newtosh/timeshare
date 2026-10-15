package trace

import (
	"bytes"
	"os"
	"testing"
)

func TestPrintfHonorsEnabled(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	t.Cleanup(func() {
		SetEnabled(false)
		SetOutput(os.Stderr)
	})

	SetEnabled(false)
	Printf("hidden %s", "x")
	if buf.Len() != 0 {
		t.Fatalf("traced while disabled: %q", buf.String())
	}

	SetEnabled(true)
	Printf("op %s", "item get")
	if got := buf.String(); got != "timeshare: op item get\n" {
		t.Fatalf("got %q", got)
	}
}

func TestEnvEnabled(t *testing.T) {
	t.Setenv("TIMESHARE_DEBUG", "")
	if EnvEnabled() {
		t.Fatal("empty env should be off")
	}
	t.Setenv("TIMESHARE_DEBUG", "0")
	if EnvEnabled() {
		t.Fatal("0 should be off")
	}
	t.Setenv("TIMESHARE_DEBUG", "1")
	if !EnvEnabled() {
		t.Fatal("1 should be on")
	}
	t.Setenv("TIMESHARE_DEBUG", "true")
	if !EnvEnabled() {
		t.Fatal("true should be on")
	}
}
