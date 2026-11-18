package monitor

import (
	"net"

	"golang.org/x/sys/unix"
)

func credentialSupervisorPID(connection *net.UnixConn) (int, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, err
	}
	var peer *unix.Ucred
	var lookupErr error
	if err := raw.Control(func(fd uintptr) {
		peer, lookupErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if lookupErr != nil {
		return 0, lookupErr
	}
	return int(peer.Pid), nil
}
