//go:build !linux && !darwin

package daemon

import (
	"fmt"
	"net"
)

// VerifyPeer is not implemented on this platform. It fails closed rather
// than silently skipping the security check.
func (s *Server) VerifyPeer(net.Conn) error {
	return fmt.Errorf("peer UID verification is not implemented on this platform; timesharedd refuses to serve without it")
}
