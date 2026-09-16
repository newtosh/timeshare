//go:build linux

package daemon

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// VerifyPeer rejects any connection from a UID other than the daemon's own
// (spec: Error handling — a second local user must not even be able to
// connect, not merely fail to authenticate).
func (s *Server) VerifyPeer(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("not a unix socket connection")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return err
	}

	var ucred *unix.Ucred
	var credErr error
	err = raw.Control(func(fd uintptr) {
		ucred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}

	if int(ucred.Uid) != os.Getuid() {
		return fmt.Errorf("connecting UID %d does not match daemon UID %d", ucred.Uid, os.Getuid())
	}
	return nil
}
