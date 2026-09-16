package daemon

import (
	"context"
	"errors"
	"log"
	"net"
	"time"

	"timeshare/internal/backend"
	"timeshare/internal/cache"
	"timeshare/internal/config"
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
	defer conn.Close()

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
	switch req.Op {
	case OpLock:
		s.Cache.Evict(req.ProjectID + "\x00")
		return Response{}
	case OpStatus:
		// v1: confirms the daemon/cache is reachable for this project; per-
		// entry TTL listing is deferred until Cache exposes an enumeration
		// method (Cache.Get/Set/Evict cover read/write/evict, not listing,
		// and adding one only for `status` isn't worth it until a second
		// caller needs it too).
		return Response{}
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

	key := req.ProjectID + "\x00" + req.SecretName
	if value, ok := s.Cache.Get(key); ok {
		return Response{Value: value}
	}

	cfg := reqToConfig(req)
	value, ttl, err := s.Backend.Resolve(context.Background(), cfg, req.SecretName)
	if err != nil {
		return Response{Error: err.Error()}
	}

	if ttl <= 0 {
		ttl = req.TTL
	}
	s.Cache.Set(key, value, ttl)
	return Response{Value: value}
}

func reqToConfig(req Request) config.Config {
	return config.Config{Vault: req.Vault, Mode: req.Mode}
}
