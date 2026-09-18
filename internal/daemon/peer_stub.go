//go:build !linux && !darwin

package daemon

import (
	"net"

	"github.com/newtosh/timeshare/internal/peercred"
)

// VerifyPeer is not implemented on this platform. It fails closed rather
// than silently skipping the security check.
func (s *Server) VerifyPeer(conn net.Conn) error {
	return peercred.Verify(conn)
}
