package daemon

import (
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/newtosh/timeshare/internal/backend"
	"github.com/newtosh/timeshare/internal/cache"
	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/trace"
	"github.com/newtosh/timeshare/internal/usermsg"
)

// ErrIdleTimeout is returned by Serve when IdleTimeout elapses with no
// accepted connections. cmd/timesharedd treats this as a clean exit, not a
// crash — the next CLI invocation respawns the daemon (spec: Components —
// daemon idle-timeout self-exit).
var ErrIdleTimeout = errors.New("daemon idle timeout reached")

// Server is the daemon's connection handler. One Server instance backs the
// whole per-user daemon process; it serves every project via the
// project_id-prefixed cache key (spec: Architecture — one daemon per OS
// user, not per project).
type Server struct {
	Cache       *cache.Cache
	Backend     backend.Backend
	IdleTimeout time.Duration // zero means never self-exit
	// Now overrides the clock for SSH grant expiry. Nil means time.Now.
	// The secret cache has its own clock (cache.New).
	Now func() time.Time
	// ResolveFingerprints overrides op fingerprint lookup in tests.
	ResolveFingerprints func(refs []string) ([]string, error)
	// UpstreamSocket overrides SSH_AUTH_SOCK discovery in tests.
	UpstreamSocket func() string

	sshMu sync.Mutex
	ssh   map[string]*sshState
}

// Serve accepts connections on ln until it errors (e.g. listener closed) or,
// if IdleTimeout is set, until that much time passes with no new connection.
func (s *Server) Serve(ln net.Listener) error {
	connCh := make(chan net.Conn)
	errCh := make(chan error, 1)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				errCh <- err
				return
			}
			connCh <- conn
		}
	}()

	idleTimer := s.newIdleTimer()
	for {
		select {
		case conn := <-connCh:
			if idleTimer != nil {
				idleTimer.Stop()
			}
			go s.HandleConn(conn)
			idleTimer = s.newIdleTimer()
		case err := <-errCh:
			return err
		case <-s.idleTimerC(idleTimer):
			// A live pass (cached secret or SSH grant) must outlive the
			// idle timer, or a 4h TTL would end at 30 minutes of quiet.
			if s.hasLiveState() {
				idleTimer = s.newIdleTimer()
				continue
			}
			return ErrIdleTimeout
		}
	}
}

func (s *Server) newIdleTimer() *time.Timer {
	if s.IdleTimeout <= 0 {
		return nil
	}
	return time.NewTimer(s.IdleTimeout)
}

func (s *Server) idleTimerC(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil // nil channel blocks forever in select, i.e. "no idle timeout"
	}
	return t.C
}

// HandleConn processes exactly one request/response exchange, then closes
// the connection. The wire protocol is one request per connection, matching
// how the CLI client operates (short-lived process per invocation).
func (s *Server) HandleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	if err := s.VerifyPeer(conn); err != nil {
		_ = WriteMessage(conn, Response{Error: "peer verification failed: " + err.Error()})
		return
	}

	var req Request
	if err := ReadMessage(conn, &req); err != nil {
		log.Printf("timesharedd: reading request: %v", err)
		return
	}

	resp := s.resolve(req)
	if err := WriteMessage(conn, resp); err != nil {
		log.Printf("timesharedd: writing response: %v", err)
	}
}

func (s *Server) resolve(req Request) Response {
	if req.Debug {
		trace.SetEnabled(true)
	}
	switch req.Op {
	case OpLock:
		if s.Cache != nil {
			s.Cache.Evict(req.ProjectID + "\x00")
		}
		s.revokeSSH(req.ProjectID)
		return Response{}
	case OpStatus:
		return Response{Passes: s.passes(req.ProjectID)}
	case OpEnsureSSH:
		return s.ensureSSH(req)
	}

	allowed := false
	for _, item := range req.AllowedItems {
		if item == req.SecretName {
			allowed = true
			break
		}
	}
	if !allowed {
		return Response{Error: "secret \"" + req.SecretName + "\" is not in this project's allow-list"}
	}

	key := req.ProjectID + "\x00" + req.SecretName + "\x00" + req.Field
	if value, ok := s.Cache.Get(key); ok {
		return Response{Value: value}
	}

	cfg := reqToConfig(req)
	value, ttl, err := s.Backend.Resolve(context.Background(), cfg, req.SecretName)
	if err != nil {
		return Response{Error: usermsg.Annotate(err).Error()}
	}

	// Config / --ttl always wins when set. Backend TTLs are only a
	// fallback for requests that omit one (e.g. older clients) — they
	// must not silently cap an 8h project TTL down to the biometric
	// CLI-session default (10m) or the service-account default (15m).
	if req.TTL > 0 {
		ttl = req.TTL
	}
	s.Cache.Set(key, value, ttl)
	return Response{Value: value}
}

func (s *Server) passes(projectID string) []Pass {
	var passes []Pass
	if s.Cache != nil && projectID != "" {
		prefix := projectID + "\x00"
		for _, m := range s.Cache.List(prefix) {
			rest := strings.TrimPrefix(m.Key, prefix)
			parts := strings.Split(rest, "\x00")
			name := parts[0]
			if len(parts) > 1 && parts[1] != "" {
				name = name + " (" + parts[1] + ")"
			}
			passes = append(passes, Pass{Kind: "item", Name: name, ExpiresAt: m.ExpiresAt})
		}
	}
	passes = append(passes, s.sshPasses(projectID)...)
	return passes
}

func (s *Server) hasLiveState() bool {
	if s.Cache != nil && len(s.Cache.List("")) > 0 {
		return true
	}
	return s.hasLiveSSH()
}

func reqToConfig(req Request) config.Config {
	return config.Config{
		Vault: req.Vault,
		Mode:  req.Mode,
		// Carry the per-request field override so backends can resolve
		// Secure Notes / custom fields without hardcoding "password".
		Items: []config.Item{{Name: req.SecretName, Field: req.Field}},
	}
}
