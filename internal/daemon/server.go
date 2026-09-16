package daemon

import (
	"context"
	"log"
	"net"

	"timeshare/internal/backend"
	"timeshare/internal/cache"
	"timeshare/internal/config"
)

// Server is the daemon's connection handler. One Server instance backs the
// whole per-user daemon process; it serves every project via the
// project_id-prefixed cache key (spec: Architecture — one daemon per OS
// user, not per project).
type Server struct {
	Cache   *cache.Cache
	Backend backend.Backend
}

// Serve accepts connections on ln until it errors (e.g. listener closed).
func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.HandleConn(conn)
	}
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
