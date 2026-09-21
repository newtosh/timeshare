package cli

import (
	"fmt"
	"testing"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"
)

// withFakeTransferItems swaps the package-level moveItem/copyItem seams for
// fakes that fail once failOnCall is reached (1-indexed call count; 0 never
// fails), restoring the real onepassword funcs afterward.
func withFakeTransferItems(t *testing.T, failOnCall int, failErr error) {
	t.Helper()
	calls := 0
	fake := func(itemName, fromVault, toVault string) error {
		calls++
		if failOnCall > 0 && calls == failOnCall {
			return failErr
		}
		return nil
	}
	origMove, origCopy := moveItem, copyItem
	moveItem = fake
	copyItem = fake
	t.Cleanup(func() { moveItem, copyItem = origMove, origCopy })
}

func itemNames(items []config.Item) []string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	return names
}

func TestTransferIntoAllSucceed(t *testing.T) {
	withFakeTransferItems(t, 0, nil)
	picked := []onepassword.Item{{ID: "1", Title: "A"}, {ID: "2", Title: "B"}}

	got, err := transferInto("dest", "src", picked, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"A", "B"}
	if names := itemNames(got); len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTransferIntoPartialFailureReturnsDoneSoFar(t *testing.T) {
	boom := fmt.Errorf("boom")
	withFakeTransferItems(t, 2, boom) // fails on the 2nd item
	picked := []onepassword.Item{{ID: "1", Title: "A"}, {ID: "2", Title: "B"}, {ID: "3", Title: "C"}}

	got, err := transferInto("dest", "src", picked, nil, false)
	if err == nil {
		t.Fatal("expected an error from the failing 2nd transfer")
	}
	// Item A transferred before the failure on B; C was never attempted.
	if names := itemNames(got); len(names) != 1 || names[0] != "A" {
		t.Fatalf("expected partial result [A], got %v", got)
	}
}

func TestTransferIntoPreservesAlreadyDone(t *testing.T) {
	withFakeTransferItems(t, 0, nil)
	picked := []onepassword.Item{{ID: "2", Title: "B"}}

	got, err := transferInto("dest", "src", picked, []config.Item{{Name: "A"}}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names := itemNames(got); len(names) != 2 || names[0] != "A" || names[1] != "B" {
		t.Fatalf("got %v, want [A B]", got)
	}
}

func TestTransferIntoDefaultsToCopyNotMove(t *testing.T) {
	var usedCopy, usedMove bool
	origMove, origCopy := moveItem, copyItem
	moveItem = func(itemName, fromVault, toVault string) error { usedMove = true; return nil }
	copyItem = func(itemName, fromVault, toVault string) error { usedCopy = true; return nil }
	t.Cleanup(func() { moveItem, copyItem = origMove, origCopy })

	if _, err := transferInto("dest", "src", []onepassword.Item{{ID: "1", Title: "A"}}, nil, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !usedCopy || usedMove {
		t.Fatalf("expected default transferInto to copy, not move (usedCopy=%v usedMove=%v)", usedCopy, usedMove)
	}
}

func TestTransferIntoMoveTrueUsesMove(t *testing.T) {
	var usedCopy, usedMove bool
	origMove, origCopy := moveItem, copyItem
	moveItem = func(itemName, fromVault, toVault string) error { usedMove = true; return nil }
	copyItem = func(itemName, fromVault, toVault string) error { usedCopy = true; return nil }
	t.Cleanup(func() { moveItem, copyItem = origMove, origCopy })

	if _, err := transferInto("dest", "src", []onepassword.Item{{ID: "1", Title: "A"}}, nil, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !usedMove || usedCopy {
		t.Fatalf("expected move=true to move, not copy (usedCopy=%v usedMove=%v)", usedCopy, usedMove)
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
