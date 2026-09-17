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

	dstItems, err := ListItems(dstVault)
	if err != nil {
		t.Fatalf("ListItems(dst): %v", err)
	}
	if len(dstItems) != 1 || dstItems[0].Title != itemName {
		t.Fatalf("expected dst vault to contain the moved item, got %v", dstItems)
	}

	// GetItem must resolve both by title and by ID (op accepts either
	// interchangeably per its own docs).
	byTitle, err := GetItem(dstVault, itemName)
	if err != nil {
		t.Fatalf("GetItem(by title): %v", err)
	}
	if byTitle.ID != dstItems[0].ID {
		t.Fatalf("GetItem(by title) returned wrong ID: got %q, want %q", byTitle.ID, dstItems[0].ID)
	}

	byID, err := GetItem(dstVault, byTitle.ID)
	if err != nil {
		t.Fatalf("GetItem(by id): %v", err)
	}
	if byID.Title != itemName {
		t.Fatalf("GetItem(by id) returned wrong title: got %q, want %q", byID.Title, itemName)
	}

	if _, err := GetItem(dstVault, "definitely-not-a-real-item-name"); err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}
