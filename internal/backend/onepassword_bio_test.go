//go:build integration

package backend

import (
	"context"
	"os"
	"testing"

	"timeshare/internal/config"
)

func TestBiometricResolvesRealSecret(t *testing.T) {
	vault := os.Getenv("TIMESHARE_TEST_VAULT")
	item := os.Getenv("TIMESHARE_TEST_ITEM")
	if vault == "" || item == "" {
		t.Skip("set TIMESHARE_TEST_VAULT, TIMESHARE_TEST_ITEM to run (requires signed-in op CLI)")
	}

	b := NewOnePasswordBiometric()
	cfg := config.Config{Vault: vault, Mode: config.ModeBiometric}

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
