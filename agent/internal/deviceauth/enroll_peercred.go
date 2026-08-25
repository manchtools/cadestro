package deviceauth

import (
	"log/slog"
	"net"
	"os"
)

func peerAuthorized(peerUID, selfUID int) bool {
	return peerUID == selfUID
}

type peerCredListener struct {
	net.Listener
	selfUID int
	logger  *slog.Logger
}

func newPeerCredListener(l net.Listener, logger *slog.Logger) *peerCredListener {
	if logger == nil {
		logger = slog.Default()
	}
	return &peerCredListener{Listener: l, selfUID: os.Getuid(), logger: logger}
}

func (l *peerCredListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		peerUID, err := peerUIDOf(conn)
		if err != nil {
			l.logger.Warn("enrollment: refusing connection; peer credentials unreadable", "error", err)
			_ = conn.Close()
			continue
		}
		if !peerAuthorized(peerUID, l.selfUID) {
			l.logger.Warn("enrollment: refusing unprivileged local caller",
				"peer_uid", peerUID, "required_uid", l.selfUID)
			_ = conn.Close()
			continue
		}
		return conn, nil
	}
}
