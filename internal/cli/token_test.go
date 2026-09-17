package cli

import (
	"strings"
	"testing"

	"github.com/newtosh/timeshare/internal/tokenstore"

	"github.com/zalando/go-keyring"
)

func TestRunTokenStoreReadsStdinAndStores(t *testing.T) {
	keyring.MockInit()

	err := runTokenStore("project-x-secrets", strings.NewReader("sa-token-value\n"))
	if err != nil {
		t.Fatalf("runTokenStore: %v", err)
	}

	got, err := tokenstore.Lookup("project-x-secrets")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != "sa-token-value" {
		t.Fatalf("got %q, want %q (whitespace should be trimmed)", got, "sa-token-value")
	}
}

func TestRunTokenStoreRejectsEmptyInput(t *testing.T) {
	keyring.MockInit()

	if err := runTokenStore("project-x-secrets", strings.NewReader("\n")); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestRunTokenDeleteRemovesToken(t *testing.T) {
	keyring.MockInit()

	if err := tokenstore.Store("project-x-secrets", "sa-token-value"); err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := runTokenDelete("project-x-secrets"); err != nil {
		t.Fatalf("runTokenDelete: %v", err)
	}

	if _, err := tokenstore.Lookup("project-x-secrets"); err == nil {
		t.Fatal("expected error after delete")
	}
}
