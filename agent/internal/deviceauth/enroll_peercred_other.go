//go:build !linux

package deviceauth

import (
	"errors"
	"net"
)

func peerUIDOf(net.Conn) (int, error) {
	return 0, errors.New("peer-credential authentication is unavailable on this platform")
}
