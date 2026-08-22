//go:build !linux

package luksd

import (
	"errors"
	"net"
)

func peerCredentialsOf(net.Conn) (peerCredentials, error) {
	return peerCredentials{}, errors.New("peer-credential authentication is unavailable on this platform")
}

func loginUIDOfPID(int) (int, error) {
	return -1, errors.New("login UID is unavailable on this platform")
}
