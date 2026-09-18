//go:build integration

package onepassword

import (
	"fmt"
	"strings"
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

	vaults, err := ListVaults()
	if err != nil {
		t.Fatalf("ListVaults: %v", err)
	}
	var sawSrc, sawDst bool
	for _, v := range vaults {
		if v.ID == srcID {
			sawSrc = true
		}
		if v.ID == dstID {
			sawDst = true
		}
	}
	if !sawSrc || !sawDst {
		t.Fatalf("expected both test vaults in ListVaults output, got %v", vaults)
	}
}

func TestCopyItemLeavesSourceInPlace(t *testing.T) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	srcVault := "timeshare-test-copysrc-" + suffix
	dstVault := "timeshare-test-copydst-" + suffix

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

	itemName := "timeshare-test-copyitem-" + suffix
	if err := CreateLoginItem(srcVault, itemName, "user", "pass"); err != nil {
		t.Fatalf("CreateLoginItem: %v", err)
	}

	if err := CopyItem(itemName, srcVault, dstVault); err != nil {
		t.Fatalf("CopyItem: %v", err)
	}

	srcItems, err := ListItems(srcVault)
	if err != nil {
		t.Fatalf("ListItems(src): %v", err)
	}
	if len(srcItems) != 1 || srcItems[0].Title != itemName {
		t.Fatalf("expected src vault to still contain the original item, got %v", srcItems)
	}

	dstItems, err := ListItems(dstVault)
	if err != nil {
		t.Fatalf("ListItems(dst): %v", err)
	}
	if len(dstItems) != 1 || dstItems[0].Title != itemName {
		t.Fatalf("expected dst vault to contain the copied item, got %v", dstItems)
	}
}

func TestGetItemFingerprintAndListSSHKeyItems(t *testing.T) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	vault := "timeshare-test-sshkey-" + suffix

	vaultID, err := CreateVault(vault)
	if err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	t.Cleanup(func() { _ = DeleteVault(vaultID) })

	itemName := "timeshare-test-sshkey-item-" + suffix
	if _, err := runOp("item", "create", "--category=SSH Key", "--title="+itemName, "--vault="+vault, "--ssh-generate-key=ed25519"); err != nil {
		t.Fatalf("creating SSH Key item: %v", err)
	}

	fp, err := GetItemFingerprint(vault, itemName)
	if err != nil {
		t.Fatalf("GetItemFingerprint: %v", err)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("expected fingerprint to start with SHA256:, got %q", fp)
	}

	items, err := ListSSHKeyItems(vault)
	if err != nil {
		t.Fatalf("ListSSHKeyItems: %v", err)
	}
	if len(items) != 1 || items[0].Title != itemName {
		t.Fatalf("expected exactly the one SSH Key item, got %v", items)
	}

	if _, err := GetItemFingerprint(vault, "definitely-not-a-real-item"); err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}
