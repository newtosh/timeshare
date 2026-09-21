package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"
)

func TestVerifyConfiguredItems(t *testing.T) {
	orig := doctorGetItem
	t.Cleanup(func() { doctorGetItem = orig })

	doctorGetItem = func(vault, ref string) (onepassword.Item, error) {
		if vault == "v" && ref == "DATABASE_URL" {
			return onepassword.Item{ID: "1", Title: ref}, nil
		}
		return onepassword.Item{}, fmt.Errorf("not found: %s", ref)
	}

	errs := verifyConfiguredItems(config.Config{Vault: "v", Items: []config.Item{{Name: "DATABASE_URL"}, {Name: "MISSING"}}})
	if len(errs) != 1 {
		t.Fatalf("errs = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "MISSING") {
		t.Fatalf("expected MISSING in error, got %v", errs)
	}
}

func TestVerifyConfiguredSSHKeys(t *testing.T) {
	orig := doctorGetSSHFingerprint
	t.Cleanup(func() { doctorGetSSHFingerprint = orig })

	doctorGetSSHFingerprint = func(vault, ref string) (string, error) {
		if vault == "Private" && ref == "deploy-key" {
			return "SHA256:abc", nil
		}
		return "", fmt.Errorf("no fingerprint")
	}

	errs := verifyConfiguredSSHKeys([]string{"Private/deploy-key", "bad-ref", "Other/missing"})
	if len(errs) != 2 {
		t.Fatalf("errs = %v", errs)
	}
}
