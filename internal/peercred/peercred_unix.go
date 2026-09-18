//go:build linux

package peercred

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// Verify rejects any connection from a UID other than the calling
// process's own — a second local user must not even be able to connect,
// not merely fail to authenticate.
func Verify(conn net.Conn) error {
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
		return fmt.Errorf("connecting UID %d does not match this process's UID %d", ucred.Uid, os.Getuid())
	}
	return nil
}
