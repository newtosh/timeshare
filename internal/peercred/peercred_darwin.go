//go:build darwin

package peercred

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// Verify rejects any connection from a UID other than the calling
// process's own. Darwin has no SO_PEERCRED; the equivalent is
// LOCAL_PEERCRED/Xucred, which carries UID/GID but not PID.
func Verify(conn net.Conn) error {
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
		return fmt.Errorf("connecting UID %d does not match this process's UID %d", xucred.Uid, os.Getuid())
	}
	return nil
}
