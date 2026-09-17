package cli

import (
	"fmt"
	"testing"

	"github.com/newtosh/timeshare/internal/onepassword"
)

// withFakeMoveItem swaps the package-level moveItem seam for a fake that
// fails once failOnCall is reached (1-indexed call count; 0 never fails),
// restoring the real onepassword.MoveItem afterward.
func withFakeMoveItem(t *testing.T, failOnCall int, failErr error) {
	t.Helper()
	calls := 0
	orig := moveItem
	moveItem = func(itemName, fromVault, toVault string) error {
		calls++
		if failOnCall > 0 && calls == failOnCall {
			return failErr
		}
		return nil
	}
	t.Cleanup(func() { moveItem = orig })
}

func TestMoveIntoAllSucceed(t *testing.T) {
	withFakeMoveItem(t, 0, nil)
	picked := []onepassword.Item{{ID: "1", Title: "A"}, {ID: "2", Title: "B"}}

	got, err := moveInto("dest", "src", picked, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"A", "B"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMoveIntoPartialFailureReturnsMovedSoFar(t *testing.T) {
	boom := fmt.Errorf("boom")
	withFakeMoveItem(t, 2, boom) // fails on the 2nd item
	picked := []onepassword.Item{{ID: "1", Title: "A"}, {ID: "2", Title: "B"}, {ID: "3", Title: "C"}}

	got, err := moveInto("dest", "src", picked, nil)
	if err == nil {
		t.Fatal("expected an error from the failing 2nd move")
	}
	// Item A moved before the failure on B; C was never attempted.
	if len(got) != 1 || got[0] != "A" {
		t.Fatalf("expected partial result [A], got %v", got)
	}
}

func TestMoveIntoPreservesAlreadyMoved(t *testing.T) {
	withFakeMoveItem(t, 0, nil)
	picked := []onepassword.Item{{ID: "2", Title: "B"}}

	got, err := moveInto("dest", "src", picked, []string{"A"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("got %v, want [A B]", got)
	}
}

func TestInitCmdHasShortFlags(t *testing.T) {
	cmd := newInitCmd()
	for long, short := range map[string]string{
		"vault": "v", "mode": "m", "ttl": "t", "item": "i", "force": "f", "non-interactive": "n",
	} {
		f := cmd.Flags().ShorthandLookup(short)
		if f == nil || f.Name != long {
			t.Errorf("expected -%s to be the shorthand for --%s, got %+v", short, long, f)
		}
	}
}
