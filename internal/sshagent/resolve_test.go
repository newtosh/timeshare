package sshagent

import (
	"fmt"
	"strings"
	"testing"
)

func TestResolveFingerprintsRejectsMissingSlash(t *testing.T) {
	if _, err := ResolveFingerprints([]string{"deploy-key-prod"}); err == nil {
		t.Fatal("expected error for a ref with no vault/ prefix")
	}
}

func TestResolveFingerprintsSplitsOnRightmostSlash(t *testing.T) {
	orig := getItemFingerprint
	defer func() { getItemFingerprint = orig }()

	var gotVault, gotItem string
	getItemFingerprint = func(vault, ref string) (string, error) {
		gotVault, gotItem = vault, ref
		return "SHA256:fake", nil
	}

	fps, err := ResolveFingerprints([]string{"team/infra/deploy-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVault != "team/infra" || gotItem != "deploy-key" {
		t.Fatalf("got vault=%q item=%q, want vault=%q item=%q", gotVault, gotItem, "team/infra", "deploy-key")
	}
	if len(fps) != 1 || fps[0] != "SHA256:fake" {
		t.Fatalf("got %v", fps)
	}
}

func TestResolveFingerprintsPropagatesError(t *testing.T) {
	orig := getItemFingerprint
	defer func() { getItemFingerprint = orig }()
	getItemFingerprint = func(vault, ref string) (string, error) {
		return "", fmt.Errorf("boom")
	}

	_, err := ResolveFingerprints([]string{"Private/deploy-key-prod"})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "Private/deploy-key-prod") {
		t.Fatalf("expected error to name the failing ref, got: %v", err)
	}
}

func TestResolveFingerprintsResolvesMultipleInOrder(t *testing.T) {
	orig := getItemFingerprint
	defer func() { getItemFingerprint = orig }()
	getItemFingerprint = func(vault, ref string) (string, error) {
		return "SHA256:" + vault + "/" + ref, nil
	}

	fps, err := ResolveFingerprints([]string{"Private/key-a", "Work/key-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"SHA256:Private/key-a", "SHA256:Work/key-b"}
	if len(fps) != 2 || fps[0] != want[0] || fps[1] != want[1] {
		t.Fatalf("got %v, want %v", fps, want)
	}
}
