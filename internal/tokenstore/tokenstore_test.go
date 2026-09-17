package tokenstore

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestStoreThenLookupHitsKeychain(t *testing.T) {
	keyring.MockInit()

	if err := Store("project-x-secrets", "sa-token-value"); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := Lookup("project-x-secrets")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != "sa-token-value" {
		t.Fatalf("got %q, want %q", got, "sa-token-value")
	}
}

func TestLookupMissingReturnsError(t *testing.T) {
	keyring.MockInit()

	if _, err := Lookup("no-such-vault"); err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestDeleteRemovesToken(t *testing.T) {
	keyring.MockInit()

	if err := Store("project-x-secrets", "sa-token-value"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := Delete("project-x-secrets"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Lookup("project-x-secrets"); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestLookupPrefersEnvVarOverKeychain(t *testing.T) {
	keyring.MockInit()

	if err := Store("project-x-secrets", "keychain-value"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	t.Setenv("TIMESHARE_SA_TOKEN_PROJECT_X_SECRETS", "env-value")

	got, err := Lookup("project-x-secrets")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != "env-value" {
		t.Fatalf("got %q, want env var to win: %q", got, "env-value")
	}
}

func TestLookupSanitizesVaultNameForEnvVar(t *testing.T) {
	keyring.MockInit()

	t.Setenv("TIMESHARE_SA_TOKEN_MY_VAULT_2", "env-value")

	got, err := Lookup("my-vault.2")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != "env-value" {
		t.Fatalf("got %q, want %q", got, "env-value")
	}
}

func TestLookupFallsBackToKeychainWhenEnvVarUnset(t *testing.T) {
	keyring.MockInit()

	if err := Store("project-x-secrets", "keychain-value"); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := Lookup("project-x-secrets")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != "keychain-value" {
		t.Fatalf("got %q, want %q", got, "keychain-value")
	}
}
