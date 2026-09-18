package sshagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
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

type signFn func(p *Proxy, key ssh.PublicKey) (*ssh.Signature, error)

func signPlain(p *Proxy, key ssh.PublicKey) (*ssh.Signature, error) {
	return p.Sign(key, []byte("data"))
}

func signWithFlags(p *Proxy, key ssh.PublicKey) (*ssh.Signature, error) {
	// flags 0: ed25519 has no algorithm choice; this only exercises Proxy guards.
	return p.SignWithFlags(key, []byte("data"), 0)
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

func TestSignGuards(t *testing.T) {
	for _, tc := range []struct {
		name string
		sign signFn
	}{
		{"Sign", signPlain},
		{"SignWithFlags", signWithFlags},
	} {
		t.Run(tc.name+"/reject", func(t *testing.T) {
			upstream := newUpstream()
			_, allowedFP := addKey(t, upstream)
			otherPub, otherFP := addKey(t, upstream)
			p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
			if _, err := tc.sign(p, otherPub); err == nil {
				t.Fatal("expected error signing with a non-allow-listed key")
			} else if !strings.Contains(err.Error(), otherFP) {
				t.Fatalf("rejection error %q does not name fingerprint %q", err, otherFP)
			}
		})
		t.Run(tc.name+"/allow", func(t *testing.T) {
			upstream := newUpstream()
			allowedPub, allowedFP := addKey(t, upstream)
			p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
			sig, err := tc.sign(p, allowedPub)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sig == nil {
				t.Fatal("expected a signature")
			}
		})
		t.Run(tc.name+"/pastDeadline", func(t *testing.T) {
			upstream := newUpstream()
			allowedPub, allowedFP := addKey(t, upstream)
			p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(-time.Second))
			if _, err := tc.sign(p, allowedPub); err == nil {
				t.Fatal("expected error signing past the deadline")
			}
		})
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

func TestExtensionReturnsUnsupported(t *testing.T) {
	p := NewProxy(newUpstream(), nil, time.Now().Add(time.Hour))
	_, err := p.Extension("foo@example.com", []byte("data"))
	if !errors.Is(err, agent.ErrExtensionUnsupported) {
		t.Fatalf("got %v, want agent.ErrExtensionUnsupported", err)
	}
}
