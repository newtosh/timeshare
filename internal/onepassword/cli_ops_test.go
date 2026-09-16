//go:build integration

package onepassword

import (
	"fmt"
	"testing"
	"time"
)

func TestCreateVaultMoveItemCleanup(t *testing.T) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	srcVault := "timeshare-test-src-" + suffix
	dstVault := "timeshare-test-dst-" + suffix

	srcID, err := CreateVault(srcVault)
	if err != nil {
		t.Fatalf("CreateVault(src): %v", err)
	}
	dstID, err := CreateVault(dstVault)
	if err != nil {
		t.Fatalf("CreateVault(dst): %v", err)
	}
	t.Cleanup(func() {
		_ = DeleteVault(srcID)
		_ = DeleteVault(dstID)
	})

	itemName := "timeshare-test-item-" + suffix
	if err := CreateLoginItem(srcVault, itemName, "user", "pass"); err != nil {
		t.Fatalf("CreateLoginItem: %v", err)
	}

	if err := MoveItem(itemName, srcVault, dstVault); err != nil {
		t.Fatalf("MoveItem: %v", err)
	}

	items, err := ListItems(srcVault)
	if err != nil {
		t.Fatalf("ListItems(src): %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected src vault empty after move, got %v", items)
	}
}
