package sshagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// addKey generates a fresh ed25519 key, adds it to kr, and returns its
// public key plus fingerprint.
func addKey(t *testing.T, kr agent.Agent) (ssh.PublicKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := kr.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return sshPub, ssh.FingerprintSHA256(sshPub)
}

func TestListFiltersToAllowedFingerprint(t *testing.T) {
	upstream := agent.NewKeyring()
	_, allowedFP := addKey(t, upstream)
	addKey(t, upstream) // a second key, never allow-listed

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	keys, err := p.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 filtered key, got %d", len(keys))
	}
	if ssh.FingerprintSHA256(keys[0]) != allowedFP {
		t.Fatalf("got fingerprint %s, want %s", ssh.FingerprintSHA256(keys[0]), allowedFP)
	}
}

func TestSignRejectsNonAllowedKey(t *testing.T) {
	upstream := agent.NewKeyring()
	_, allowedFP := addKey(t, upstream)
	otherPub, _ := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	if _, err := p.Sign(otherPub, []byte("data")); err == nil {
		t.Fatal("expected error signing with a non-allow-listed key")
	}
}

func TestSignAllowsAllowedKeyBeforeDeadline(t *testing.T) {
	upstream := agent.NewKeyring()
	allowedPub, allowedFP := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	sig, err := p.Sign(allowedPub, []byte("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected a signature")
	}
}

func TestSignFailsPastDeadline(t *testing.T) {
	upstream := agent.NewKeyring()
	allowedPub, allowedFP := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(-time.Second))
	if _, err := p.Sign(allowedPub, []byte("data")); err == nil {
		t.Fatal("expected error signing past the deadline")
	}
}

func TestListEmptyPastDeadline(t *testing.T) {
	upstream := agent.NewKeyring()
	addKey(t, upstream)

	p := NewProxy(upstream, nil, time.Now().Add(-time.Second))
	keys, err := p.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys past deadline, got %d", len(keys))
	}
}

func TestUnsupportedMethodsReturnError(t *testing.T) {
	p := NewProxy(agent.NewKeyring(), nil, time.Now().Add(time.Hour))
	if err := p.Add(agent.AddedKey{}); err == nil {
		t.Error("expected Add to fail")
	}
	if err := p.RemoveAll(); err == nil {
		t.Error("expected RemoveAll to fail")
	}
	if _, err := p.Signers(); err == nil {
		t.Error("expected Signers to fail")
	}
}
