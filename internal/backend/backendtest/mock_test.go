package backendtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"timeshare/internal/backend"
	"timeshare/internal/config"
)

func TestMockResolvesConfiguredValue(t *testing.T) {
	m := &Mock{
		ValueFor: map[string]string{"DATABASE_URL": "postgres://x"},
		TTL:      time.Hour,
	}
	cfg := config.Config{Vault: "v", Mode: config.ModeServiceAccount}

	val, ttl, err := m.Resolve(context.Background(), cfg, "DATABASE_URL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "postgres://x" || ttl != time.Hour {
		t.Fatalf("got (%q, %v)", val, ttl)
	}
	if len(m.Calls) != 1 || m.Calls[0] != "DATABASE_URL" {
		t.Fatalf("expected call recorded, got %v", m.Calls)
	}
}

func TestMockReturnsConfiguredError(t *testing.T) {
	wantErr := errors.New("auth failed")
	m := &Mock{Err: wantErr}
	cfg := config.Config{}

	_, _, err := m.Resolve(context.Background(), cfg, "ANYTHING")
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
}

func TestMockSatisfiesBackendInterface(t *testing.T) {
	var _ backend.Backend = &Mock{}
}
