// Package sshagent implements a filtering ssh-agent protocol proxy: it
// forwards List/Sign/SignWithFlags requests to a real upstream agent, but
// only for a fixed set of allow-listed key fingerprints, and only before
// a fixed deadline. It never holds private key material — every Sign
// call that passes the filter forwards the raw request upstream and
// relays the signature back unmodified.
package sshagent

import (
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/newtosh/timeshare/internal/peercred"
)

// ErrNotSupported is returned by every Proxy method that would manage
// agent state (Add/Remove/RemoveAll/Lock/Unlock/Signers) — this is a
// read-only, filtering pass-through to an upstream agent, never a key
// store of its own.
var ErrNotSupported = errors.New("timeshare: sshagent proxy does not support this operation")

// Proxy implements agent.ExtendedAgent as a filtering pass-through to an
// upstream agent (the real 1Password SSH agent in production, a fake
// keyring in tests). ExtendedAgent (not just Agent) matters because the
// ssh-agent protocol server drops the client's requested signature flags
// entirely when the agent it wraps doesn't implement SignWithFlags — for
// an RSA key that silently downgrades every signature to legacy
// ssh-rsa/SHA-1, which modern OpenSSH servers reject.
type Proxy struct {
	upstream agent.ExtendedAgent
	allowed  map[string]bool
	deadline time.Time
}

// NewProxy builds a Proxy that forwards to upstream, permitting only the
// given fingerprints (e.g. "SHA256:...", exactly as returned by
// ssh.FingerprintSHA256 or by 1Password's own "fingerprint" item field —
// the two formats are identical, byte for byte) until deadline.
func NewProxy(upstream agent.ExtendedAgent, fingerprints []string, deadline time.Time) *Proxy {
	allowed := make(map[string]bool, len(fingerprints))
	for _, fp := range fingerprints {
		allowed[fp] = true
	}
	return &Proxy{upstream: upstream, allowed: allowed, deadline: deadline}
}

// List returns only the upstream identities whose fingerprint is
// allow-listed. Past deadline, it returns an empty list — mirrors the
// agent.Agent interface's own documented Lock() behavior ("List will
// empty an empty list").
func (p *Proxy) List() ([]*agent.Key, error) {
	if !time.Now().Before(p.deadline) {
		return nil, nil
	}
	keys, err := p.upstream.List()
	if err != nil {
		return nil, err
	}
	var filtered []*agent.Key
	for _, k := range keys {
		if p.allowed[ssh.FingerprintSHA256(k)] {
			filtered = append(filtered, k)
		}
	}
	return filtered, nil
}

// Sign forwards to the upstream agent only if key's fingerprint is
// allow-listed and the deadline hasn't passed; otherwise it fails
// without ever contacting upstream.
func (p *Proxy) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	if !time.Now().Before(p.deadline) {
		return nil, errors.New("timeshare: SSH key grant TTL expired")
	}
	fingerprint := ssh.FingerprintSHA256(key)
	if !p.allowed[fingerprint] {
		return nil, fmt.Errorf("timeshare: SSH key %s not in this repo's allow-list", fingerprint)
	}
	return p.upstream.Sign(key, data)
}

// SignWithFlags signs like Sign, but allows the client to request a
// specific signature algorithm (e.g. rsa-sha2-256 instead of the legacy
// ssh-rsa/SHA-1) — same allow-list and deadline guards as Sign, then
// forwards to the upstream ExtendedAgent unmodified.
func (p *Proxy) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	if !time.Now().Before(p.deadline) {
		return nil, errors.New("timeshare: SSH key grant TTL expired")
	}
	fingerprint := ssh.FingerprintSHA256(key)
	if !p.allowed[fingerprint] {
		return nil, fmt.Errorf("timeshare: SSH key %s not in this repo's allow-list", fingerprint)
	}
	return p.upstream.SignWithFlags(key, data, flags)
}

// Extension is not supported — this proxy only implements the standard
// List/Sign/SignWithFlags surface.
func (p *Proxy) Extension(extensionType string, contents []byte) ([]byte, error) {
	return nil, agent.ErrExtensionUnsupported
}

func (p *Proxy) Add(agent.AddedKey) error       { return ErrNotSupported }
func (p *Proxy) Remove(ssh.PublicKey) error     { return ErrNotSupported }
func (p *Proxy) RemoveAll() error               { return ErrNotSupported }
func (p *Proxy) Lock([]byte) error              { return ErrNotSupported }
func (p *Proxy) Unlock([]byte) error            { return ErrNotSupported }
func (p *Proxy) Signers() ([]ssh.Signer, error) { return nil, ErrNotSupported }

// Serve accepts connections on l and serves the ssh-agent protocol using
// p for each one, until l.Accept fails (typically because the caller
// closed l on shutdown). Each connection is served in its own goroutine
// so one slow or stuck client can't block another.
func Serve(l net.Listener, p *Proxy) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer func() { _ = conn.Close() }()
			if err := peercred.Verify(conn); err != nil {
				log.Printf("sshagent: rejecting connection: %v", err)
				return
			}
			_ = agent.ServeAgent(p, conn)
		}()
	}
}
