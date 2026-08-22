//go:build linux

package luksd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func peerCredentialsOf(conn net.Conn) (peerCredentials, error) {
	if addr := conn.LocalAddr(); addr == nil || addr.Network() != "unix" {
		return peerCredentials{}, errors.New("peer credentials are only available on a unix-domain connection")
	}
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return peerCredentials{}, errors.New("connection does not expose a syscall.Conn")
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return peerCredentials{}, err
	}
	var ucred *unix.Ucred
	var credErr error
	if ctrlErr := raw.Control(func(fd uintptr) {
		ucred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); ctrlErr != nil {
		return peerCredentials{}, ctrlErr
	}
	if credErr != nil {
		return peerCredentials{}, credErr
	}
	if ucred == nil {
		return peerCredentials{}, errors.New("peer credentials were not returned")
	}
	return peerCredentials{uid: int(ucred.Uid), pid: int(ucred.Pid)}, nil
}

func loginUIDOfPID(pid int) (int, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/loginuid", pid))
	if err != nil {
		return -1, err
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err != nil {
		return -1, fmt.Errorf("parse loginuid: %w", err)
	}
	if value == uint64(^uint32(0)) {
		return -1, nil
	}
	return int(value), nil
}
