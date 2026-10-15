package daemon

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/ssh/agent"

	"github.com/newtosh/timeshare/internal/sshagent"
	"github.com/newtosh/timeshare/internal/trace"
	"github.com/newtosh/timeshare/internal/usermsg"
)

// sshState is one project's SSH pass: the resolved fingerprints (not
// secrets) and, once the upstream agent is reachable, the proxy socket.
// Both live only in daemon memory. Lock drops them.
type sshState struct {
	mu           sync.Mutex
	refs         []string
	fingerprints []string
	expiresAt    time.Time
	live         *liveProxy
}

type liveProxy struct {
	socketPath string
	ln         net.Listener
	upstream   net.Conn
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) state(projectID string) *sshState {
	s.sshMu.Lock()
	defer s.sshMu.Unlock()
	if s.ssh == nil {
		s.ssh = make(map[string]*sshState)
	}
	st := s.ssh[projectID]
	if st == nil {
		st = &sshState{}
		s.ssh[projectID] = st
	}
	return st
}

func (s *Server) ensureSSH(req Request) Response {
	if req.ProjectID == "" {
		return Response{Error: "ssh grant requires a project id"}
	}
	if len(req.SSHKeys) == 0 {
		return Response{Error: "ssh grant requires at least one ssh key"}
	}
	if req.TTL <= 0 {
		return Response{Error: "ssh grant requires a ttl"}
	}

	st := s.state(req.ProjectID)
	st.mu.Lock()
	defer st.mu.Unlock()

	now := s.now()
	trace.Printf("ensure ssh project=%s keys=%d", req.ProjectID, len(req.SSHKeys))

	if st.expiresAt.IsZero() || !now.Before(st.expiresAt) || !sameRefs(st.refs, req.SSHKeys) {
		fps, err := s.fingerprints(req.SSHKeys)
		if err != nil {
			return Response{Error: usermsg.Annotate(err).Error()}
		}
		// TTL starts at the approval (a successful fingerprint
		// resolution), not at the moment a later run reuses the grant.
		st.refs = append([]string(nil), req.SSHKeys...)
		st.fingerprints = fps
		st.expiresAt = now.Add(req.TTL)
		st.closeLive()
		trace.Printf("ssh grant established project=%s fingerprints=%d expires=%s", req.ProjectID, len(fps), st.expiresAt.Format(time.RFC3339))
	} else {
		trace.Printf("ssh grant reused project=%s expires=%s", req.ProjectID, st.expiresAt.Format(time.RFC3339))
	}

	if st.live != nil {
		if _, err := os.Stat(st.live.socketPath); err == nil {
			return Response{SocketPath: st.live.socketPath, ExpiresAt: st.expiresAt}
		}
		st.closeLive()
	}

	if err := s.listenProxy(req.ProjectID, st); err != nil {
		// Fingerprints stay cached so a retry does not prompt again.
		return Response{Error: usermsg.Annotate(err).Error()}
	}
	return Response{SocketPath: st.live.socketPath, ExpiresAt: st.expiresAt}
}

func (s *Server) fingerprints(refs []string) ([]string, error) {
	if s.ResolveFingerprints != nil {
		return s.ResolveFingerprints(refs)
	}
	return sshagent.ResolveFingerprints(refs)
}

func (s *Server) upstreamPath() string {
	if s.UpstreamSocket != nil {
		return s.UpstreamSocket()
	}
	return sshagent.UpstreamSocketPath()
}

func (s *Server) listenProxy(projectID string, st *sshState) error {
	path := sshagent.ProxySocketPath(projectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	_ = os.Remove(path)

	upPath := s.upstreamPath()
	if upPath == "" {
		return fmt.Errorf("could not determine the upstream SSH agent socket path (set SSH_AUTH_SOCK)")
	}
	conn, err := net.DialTimeout("unix", upPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to upstream SSH agent at %s: %w", upPath, err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("creating SSH proxy socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = conn.Close()
		_ = os.Remove(path)
		return fmt.Errorf("securing SSH proxy socket: %w", err)
	}

	proxy := sshagent.NewProxy(agent.NewClient(conn), st.fingerprints, st.expiresAt)
	go func() { _ = sshagent.Serve(ln, proxy) }()
	st.live = &liveProxy{socketPath: path, ln: ln, upstream: conn}
	trace.Printf("ssh proxy listening path=%s", path)
	return nil
}

func (st *sshState) closeLive() {
	if st.live == nil {
		return
	}
	if st.live.ln != nil {
		_ = st.live.ln.Close()
	}
	if st.live.upstream != nil {
		_ = st.live.upstream.Close()
	}
	if st.live.socketPath != "" {
		_ = os.Remove(st.live.socketPath)
	}
	st.live = nil
}

func (st *sshState) clear() {
	st.closeLive()
	st.refs = nil
	st.fingerprints = nil
	st.expiresAt = time.Time{}
}

func (s *Server) revokeSSH(projectID string) {
	s.sshMu.Lock()
	st := s.ssh[projectID]
	s.sshMu.Unlock()
	if st == nil {
		return
	}
	st.mu.Lock()
	st.clear()
	st.mu.Unlock()
}

func (s *Server) sshPasses(projectID string) []Pass {
	s.sshMu.Lock()
	st := s.ssh[projectID]
	s.sshMu.Unlock()
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.fingerprints) == 0 || !s.now().Before(st.expiresAt) {
		return nil
	}
	refs := append([]string(nil), st.refs...)
	sort.Strings(refs)
	out := make([]Pass, 0, len(refs))
	for _, ref := range refs {
		out = append(out, Pass{Kind: "ssh", Name: ref, ExpiresAt: st.expiresAt})
	}
	return out
}

func (s *Server) hasLiveSSH() bool {
	now := s.now()
	s.sshMu.Lock()
	defer s.sshMu.Unlock()
	for _, st := range s.ssh {
		st.mu.Lock()
		alive := len(st.fingerprints) > 0 && now.Before(st.expiresAt)
		st.mu.Unlock()
		if alive {
			return true
		}
	}
	return false
}

func sameRefs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
