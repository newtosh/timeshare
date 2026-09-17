package daemon

import (
	"bytes"
	"testing"
	"time"

	"github.com/newtosh/timeshare/internal/config"
)

func TestRequestRoundTrip(t *testing.T) {
	want := Request{
		ProjectID:    "abc123",
		SecretName:   "DATABASE_URL",
		Vault:        "project-x-secrets",
		Mode:         config.ModeServiceAccount,
		TTL:          4 * time.Hour,
		AllowedItems: []string{"DATABASE_URL", "STRIPE_KEY"},
	}

	var buf bytes.Buffer
	if err := WriteMessage(&buf, want); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got Request
	if err := ReadMessage(&buf, &got); err != nil {
		t.Fatalf("read: %v", err)
	}
	// Request has only comparable fields except AllowedItems (a slice);
	// compare that separately.
	if got.ProjectID != want.ProjectID || got.SecretName != want.SecretName {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got.Vault != want.Vault || got.Mode != want.Mode || got.TTL != want.TTL {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if len(got.AllowedItems) != 2 || got.AllowedItems[1] != "STRIPE_KEY" {
		t.Fatalf("AllowedItems mismatch: %v", got.AllowedItems)
	}
}

func TestResponseRoundTrip(t *testing.T) {
	want := Response{Value: "secret-value", ExpiresAt: time.Now().Truncate(time.Second)}

	var buf bytes.Buffer
	if err := WriteMessage(&buf, want); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got Response
	if err := ReadMessage(&buf, &got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Value != want.Value || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMultipleMessagesOnOneStream(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteMessage(&buf, Request{SecretName: "A"})
	_ = WriteMessage(&buf, Request{SecretName: "B"})

	var first, second Request
	if err := ReadMessage(&buf, &first); err != nil {
		t.Fatal(err)
	}
	if err := ReadMessage(&buf, &second); err != nil {
		t.Fatal(err)
	}
	if first.SecretName != "A" || second.SecretName != "B" {
		t.Fatalf("got %q then %q", first.SecretName, second.SecretName)
	}
}
