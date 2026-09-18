//go:build darwin

package daemon

import (
	"net"

	"github.com/newtosh/timeshare/internal/peercred"
)

// VerifyPeer rejects any connection from a UID other than the daemon's own
// (spec: Error handling — a second local user must not even be able to
// connect, not merely fail to authenticate). Darwin has no SO_PEERCRED;
// the equivalent is LOCAL_PEERCRED/Xucred, which carries UID/GID but not
// PID (that would need a separate LOCAL_PEERPID call, unneeded here).
func (s *Server) VerifyPeer(conn net.Conn) error {
	return peercred.Verify(conn)
}
