package backend

import (
	"testing"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"
)

func TestBiometricResolveUsesReadField(t *testing.T) {
	orig := readField
	t.Cleanup(func() { readField = orig })

	var gotVault, gotItem, gotField string
	readField = func(vault, item, field string) (string, error) {
		gotVault, gotItem, gotField = vault, item, field
		return "secret-value", nil
	}

	b := NewOnePasswordBiometric()
	val, ttl, err := b.Resolve(t.Context(), config.Config{Vault: "v", Mode: config.ModeBiometric}, "Team/Prod DB")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret-value" || ttl != defaultBiometricTTL {
		t.Fatalf("val=%q ttl=%v", val, ttl)
	}
	if gotVault != "v" || gotItem != "Team/Prod DB" || gotField != onepassword.DefaultSecretField {
		t.Fatalf("readField args = %q %q %q", gotVault, gotItem, gotField)
	}
}
