//go:build integration

package backend

import (
	"context"
	"os"
	"testing"

	"github.com/newtosh/timeshare/internal/config"
)

func TestServiceAccountResolvesRealSecret(t *testing.T) {
	token := os.Getenv("TIMESHARE_TEST_SA_TOKEN")
	vault := os.Getenv("TIMESHARE_TEST_VAULT")
	item := os.Getenv("TIMESHARE_TEST_ITEM")
	if token == "" || vault == "" || item == "" {
		t.Skip("set TIMESHARE_TEST_SA_TOKEN, TIMESHARE_TEST_VAULT, TIMESHARE_TEST_ITEM to run")
	}

	b := NewOnePasswordServiceAccount(token)
	cfg := config.Config{Vault: vault, Mode: config.ModeServiceAccount}

	val, ttl, err := b.Resolve(context.Background(), cfg, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == "" {
		t.Fatal("expected non-empty secret value")
	}
	if ttl <= 0 {
		t.Fatal("expected a positive suggested TTL")
	}
}

func TestServiceAccountRejectsBadToken(t *testing.T) {
	b := NewOnePasswordServiceAccount("invalid-token")
	cfg := config.Config{Vault: "whatever"}
	_, _, err := b.Resolve(context.Background(), cfg, "X")
	if err == nil {
		t.Fatal("expected auth error for invalid token")
	}
}
