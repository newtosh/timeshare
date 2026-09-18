//go:build !linux && !darwin

package peercred

import (
	"fmt"
	"net"
)

// Verify is not implemented on this platform. It fails closed rather
// than silently skipping the security check.
func Verify(net.Conn) error {
	return fmt.Errorf("peer UID verification is not implemented on this platform")
}
