package luksd

import (
	"log/slog"
	"net"
	"os"
)

type peerCredentials struct {
	uid int
	pid int
}

func peerAuthorized(peerUID, selfUID, loginUID int) bool {
	if peerUID < 0 {
		return false
	}
	if peerUID == selfUID {
		return true
	}
	return loginUID >= 0 && peerUID == loginUID
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
		credentials, err := peerCredentialsOf(conn)
		if err != nil {
			l.logger.Warn("luksd: refusing connection; peer credentials unreadable", "error", err)
			_ = conn.Close()
			continue
		}
		loginUID := -1
		if credentials.uid != l.selfUID {
			loginUID, err = loginUIDOfPID(credentials.pid)
			if err != nil {
				l.logger.Warn("luksd: refusing connection; login identity unreadable", "error", err)
				_ = conn.Close()
				continue
			}
		}
		if !peerAuthorized(credentials.uid, l.selfUID, loginUID) {
			l.logger.Warn("luksd: refusing LUKS passphrase request from a non-login uid",
				"peer_uid", credentials.uid, "login_uid", loginUID, "agent_uid", l.selfUID)
			_ = conn.Close()
			continue
		}
		return conn, nil
	}
}
