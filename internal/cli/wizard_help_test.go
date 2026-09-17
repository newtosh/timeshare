package cli

import "testing"

func TestIsHelpRequest(t *testing.T) {
	cases := map[string]bool{
		"?":   true,
		" ? ": true,
		"":    false,
		"4h":  false,
	}
	for input, want := range cases {
		if got := isHelpRequest(input); got != want {
			t.Errorf("isHelpRequest(%q) = %v, want %v", input, got, want)
		}
	}
}
