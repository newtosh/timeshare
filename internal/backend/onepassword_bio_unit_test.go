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

func TestBiometricResolveUsesConfiguredField(t *testing.T) {
	orig := readField
	t.Cleanup(func() { readField = orig })

	var gotField string
	readField = func(vault, item, field string) (string, error) {
		gotField = field
		return "token", nil
	}

	b := NewOnePasswordBiometric()
	cfg := config.Config{
		Vault: "v",
		Mode:  config.ModeBiometric,
		Items: []config.Item{{Name: "sbg-engtools.gen", Field: "notesPlain"}},
	}
	if _, _, err := b.Resolve(t.Context(), cfg, "sbg-engtools.gen"); err != nil {
		t.Fatal(err)
	}
	if gotField != "notesPlain" {
		t.Fatalf("got field %q, want notesPlain", gotField)
	}
}
