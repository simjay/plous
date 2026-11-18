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
	var pid int
	var lookupErr error
	if err := raw.Control(func(fd uintptr) {
		pid, lookupErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	}); err != nil {
		return 0, err
	}
	return pid, lookupErr
}
