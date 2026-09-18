package sshagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// newUpstream returns a fresh in-memory keyring as an agent.ExtendedAgent.
// agent.NewKeyring's declared return type is agent.Agent, but its
// concrete *keyring type also implements SignWithFlags/Extension, so the
// assertion always succeeds — this stands in for the real upstream,
// which is always an agent.ExtendedAgent via agent.NewClient.
func newUpstream() agent.ExtendedAgent {
	return agent.NewKeyring().(agent.ExtendedAgent)
}

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
	upstream := newUpstream()
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
	upstream := newUpstream()
	_, allowedFP := addKey(t, upstream)
	otherPub, _ := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	if _, err := p.Sign(otherPub, []byte("data")); err == nil {
		t.Fatal("expected error signing with a non-allow-listed key")
	}
}

func TestSignAllowsAllowedKeyBeforeDeadline(t *testing.T) {
	upstream := newUpstream()
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
	upstream := newUpstream()
	allowedPub, allowedFP := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(-time.Second))
	if _, err := p.Sign(allowedPub, []byte("data")); err == nil {
		t.Fatal("expected error signing past the deadline")
	}
}

func TestListEmptyPastDeadline(t *testing.T) {
	upstream := newUpstream()
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
	p := NewProxy(newUpstream(), nil, time.Now().Add(time.Hour))
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

func TestSignWithFlagsRejectsNonAllowedKey(t *testing.T) {
	upstream := newUpstream()
	_, allowedFP := addKey(t, upstream)
	otherPub, _ := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	if _, err := p.SignWithFlags(otherPub, []byte("data"), agent.SignatureFlagRsaSha256); err == nil {
		t.Fatal("expected error signing with a non-allow-listed key")
	}
}

func TestSignWithFlagsAllowsAllowedKeyBeforeDeadline(t *testing.T) {
	upstream := newUpstream()
	allowedPub, allowedFP := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	// flags 0 here: the test key is ed25519, which has no algorithm
	// choice to make (unlike RSA), so this only exercises the Proxy's
	// own allow-list/deadline guards and forwarding — not flag
	// interpretation, which is the trusted upstream library's job.
	sig, err := p.SignWithFlags(allowedPub, []byte("data"), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected a signature")
	}
}

func TestSignWithFlagsFailsPastDeadline(t *testing.T) {
	upstream := newUpstream()
	allowedPub, allowedFP := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(-time.Second))
	if _, err := p.SignWithFlags(allowedPub, []byte("data"), agent.SignatureFlagRsaSha256); err == nil {
		t.Fatal("expected error signing past the deadline")
	}
}

func TestExtensionReturnsUnsupported(t *testing.T) {
	p := NewProxy(newUpstream(), nil, time.Now().Add(time.Hour))
	_, err := p.Extension("foo@example.com", []byte("data"))
	if !errors.Is(err, agent.ErrExtensionUnsupported) {
		t.Fatalf("got %v, want agent.ErrExtensionUnsupported", err)
	}
}
