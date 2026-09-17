package cli

import (
	"testing"
	"time"
)

func TestFormatTTL(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{4 * time.Hour, "4h"},
		{90 * time.Minute, "1h30m"},
		{30 * time.Minute, "30m"},
		{45 * time.Second, "45s"},
		{0, "0s"},
		{time.Minute, "1m"},
	}
	for _, c := range cases {
		if got := formatTTL(c.in); got != c.want {
			t.Errorf("formatTTL(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
