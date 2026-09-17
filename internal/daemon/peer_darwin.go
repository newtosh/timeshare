//go:build darwin

package daemon

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// VerifyPeer rejects any connection from a UID other than the daemon's own
// (spec: Error handling — a second local user must not even be able to
// connect, not merely fail to authenticate). Darwin has no SO_PEERCRED;
// the equivalent is LOCAL_PEERCRED/Xucred, which carries UID/GID but not
// PID (that would need a separate LOCAL_PEERPID call, unneeded here).
func (s *Server) VerifyPeer(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("not a unix socket connection")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return err
	}

	var xucred *unix.Xucred
	var credErr error
	err = raw.Control(func(fd uintptr) {
		xucred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})
	if err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}
	if xucred.Ngroups == 0 {
		return fmt.Errorf("LOCAL_PEERCRED returned no groups, refusing to trust peer")
	}

	if int(xucred.Uid) != os.Getuid() {
		return fmt.Errorf("connecting UID %d does not match daemon UID %d", xucred.Uid, os.Getuid())
	}
	return nil
}
