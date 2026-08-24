package luksd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/manchtools/cadestro/agent/internal/store"
	sdk "github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysenc "github.com/manchtools/cadestro/sdk/sys/encryption"
)

type Session interface {
	ValidateLuksToken(ctx context.Context, token string) (*sdk.ValidateLuksTokenResult, error)
	GetLuksKey(ctx context.Context, actionID string) (string, error)
}

type StateStore interface {
	GetLuksState(actionID string) (*store.LuksState, error)
	GetLuksPassphraseHashes(actionID string) ([]string, error)
	SetLuksDeviceKeyType(actionID, keyType string) error
	AddLuksPassphraseHash(actionID, hash string) error
}

type Enroller interface {
	AddKeyToSlot(ctx context.Context, devicePath string, slot int, unlockKey, newKey string) error
	KillSlot(ctx context.Context, devicePath string, slot int, unlockKey string) error
	WipeTPM(ctx context.Context, devicePath, unlockKey string) error
}

type Daemon struct {
	socketPath string
	logger     *slog.Logger
	store      StateStore
	enroller   Enroller

	mu      sync.RWMutex
	session Session

	listenerMu sync.Mutex
	listener   net.Listener
	wg         sync.WaitGroup

	inFlight chan struct{}

	now func() time.Time
}

func NewDaemon(socketPath string, st StateStore, enroller Enroller, logger *slog.Logger) *Daemon {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Daemon{
		socketPath: socketPath,
		logger:     logger,
		store:      st,
		enroller:   enroller,
		inFlight:   make(chan struct{}, maxConcurrentRequests),
		now:        time.Now,
	}
}

func (d *Daemon) SetSession(s Session) {
	d.mu.Lock()
	d.session = s
	d.mu.Unlock()
}

func (d *Daemon) ClearSession() {
	d.mu.Lock()
	d.session = nil
	d.mu.Unlock()
}

func (d *Daemon) currentSession() Session {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.session
}

const (
	maxConcurrentRequests = 4
	requestTimeout        = 90 * time.Second
	busyWriteTimeout      = 2 * time.Second
)

func (d *Daemon) Start(ctx context.Context) error {
	dir := filepath.Dir(d.socketPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create socket directory %s: %w", dir, err)
	}
	_ = os.Remove(d.socketPath)

	listener, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.socketPath, err)
	}
	if err := os.Chmod(d.socketPath, 0o622); err != nil {
		_ = listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	guarded := newPeerCredListener(listener, d.logger)

	d.listenerMu.Lock()
	d.listener = listener
	d.listenerMu.Unlock()

	d.logger.Info("LUKS passphrase daemon listening", "socket", d.socketPath)

	go func() {
		<-ctx.Done()
		d.Shutdown()
	}()

	for {
		conn, err := guarded.Accept()
		if err != nil {
			d.wg.Wait()
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			d.logger.Error("LUKS daemon accept failed; daemon stopping", "error", err)
			return fmt.Errorf("luksd accept: %w", err)
		}
		select {
		case d.inFlight <- struct{}{}:
		default:
			d.logger.Warn("LUKS daemon: refusing request; too many in flight", "limit", maxConcurrentRequests)
			_ = conn.SetWriteDeadline(d.now().Add(busyWriteTimeout))
			d.writeResponse(conn, errResponse(CodeBusy, "too many concurrent LUKS requests; retry shortly"))
			_ = conn.Close()
			continue
		}
		d.wg.Add(1)
		go func() {
			defer func() {
				<-d.inFlight
				d.wg.Done()
			}()
			d.handleConn(ctx, conn)
		}()
	}
}

func (d *Daemon) Shutdown() {
	d.listenerMu.Lock()
	l := d.listener
	d.listener = nil
	d.listenerMu.Unlock()
	if l != nil {
		_ = l.Close()
	}
	_ = os.Remove(d.socketPath)
}

func (d *Daemon) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(d.now().Add(30 * time.Second))

	var req Request
	dec := json.NewDecoder(io.LimitReader(conn, maxRequestBytes))
	if err := dec.Decode(&req); err != nil {
		d.writeResponse(conn, Response{OK: false, Code: CodeInternal, Error: "malformed request"})
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp := d.handleRequest(reqCtx, req)
	_ = conn.SetWriteDeadline(d.now().Add(10 * time.Second))
	d.writeResponse(conn, resp)
}

func (d *Daemon) writeResponse(conn net.Conn, resp Response) {
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		d.logger.Warn("failed to write LUKS daemon response", "error", err)
	}
}

const maxRequestBytes = 64 * 1024

func errResponse(code, msg string) Response {
	return Response{OK: false, Code: code, Error: msg}
}

func (d *Daemon) handleRequest(ctx context.Context, req Request) Response {
	if req.Token == "" {
		return errResponse(CodeMissingToken, "token is required")
	}

	sess := d.currentSession()
	if sess == nil {
		return errResponse(CodeNotConnected, "agent is not connected to the control; retry when online")
	}
	result, err := sess.ValidateLuksToken(ctx, req.Token)
	if err != nil {
		d.logger.Warn("LUKS daemon: token validation failed", "error", err)
		return errResponse(CodeInvalidToken, "token is invalid or has expired")
	}

	complexity := mapComplexity(result.Complexity)
	minLen := int(result.MinLength)
	if minLen < minPassphraseLength {
		minLen = minPassphraseLength
	}
	if vErr := sysenc.ValidatePassphrase(req.Passphrase, minLen, complexity); vErr != "" {
		return errResponse(CodePassphrasePolicy, vErr)
	}

	recent, err := d.store.GetLuksPassphraseHashes(result.ActionID)
	if err != nil {
		d.logger.Warn("LUKS daemon: failed to read passphrase history", "action_id", result.ActionID, "error", err)
		return errResponse(CodeInternal, "failed to check passphrase history")
	}
	if sysenc.IsRecentlyUsed(req.Passphrase, recent) {
		return errResponse(CodePassphraseReuse, "this passphrase was used recently; choose a different one")
	}

	managedKey, err := sess.GetLuksKey(ctx, result.ActionID)
	if err != nil {
		d.logger.Warn("LUKS daemon: failed to fetch managed key", "action_id", result.ActionID, "error", err)
		return errResponse(CodeKeyUnavailable, "failed to fetch the managed key")
	}

	localState, err := d.store.GetLuksState(result.ActionID)
	if err != nil {
		d.logger.Error("LUKS daemon: failed to read local state", "action_id", result.ActionID, "error", err)
		return errResponse(CodeInternal, "failed to read local LUKS state")
	}
	revoked := false
	if localState != nil && localState.DeviceKeyType != "none" && localState.DeviceKeyType != "" {
		switch localState.DeviceKeyType {
		case "tpm":
			if err := d.enroller.WipeTPM(ctx, result.DevicePath, managedKey); err != nil {
				d.logger.Error("luksd: remove existing TPM key failed", "device", result.DevicePath, "error", err)
				return errResponse(CodeInternal, "failed to remove existing TPM key")
			}
			revoked = true
		case "user_passphrase":
			if err := d.enroller.KillSlot(ctx, result.DevicePath, userPassphraseSlot, managedKey); err != nil {
				d.logger.Error("luksd: remove existing passphrase failed", "device", result.DevicePath, "error", err)
				return errResponse(CodeInternal, "failed to remove existing passphrase")
			}
			revoked = true
		}
	}

	if err := d.enroller.AddKeyToSlot(ctx, result.DevicePath, userPassphraseSlot, managedKey, req.Passphrase); err != nil {
		d.logger.Error("luksd: set passphrase failed", "device", result.DevicePath, "error", err)
		if revoked {
			if serr := d.store.SetLuksDeviceKeyType(result.ActionID, "none"); serr != nil {
				d.logger.Error("luksd: failed to record emptied key slot after failed enroll", "action_id", result.ActionID, "error", serr)
			}
		}
		return errResponse(CodeInternal, "failed to set passphrase")
	}

	if err := d.store.SetLuksDeviceKeyType(result.ActionID, "user_passphrase"); err != nil {
		d.logger.Error("LUKS daemon: failed to persist device key type", "action_id", result.ActionID, "error", err)
		return errResponse(CodeInternal, "passphrase was set but local state update failed; rerun to recover")
	}
	if err := d.store.AddLuksPassphraseHash(result.ActionID, sysenc.HashPassphrase(req.Passphrase)); err != nil {
		d.logger.Error("LUKS daemon: failed to persist passphrase history", "action_id", result.ActionID, "error", err)
		return errResponse(CodeInternal, "passphrase was set but history update failed")
	}

	return Response{OK: true, Code: CodeOK}
}

func mapComplexity(c cadestrov1.LpsPasswordComplexity) sysenc.Complexity {
	switch c {
	case cadestrov1.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_ALPHANUMERIC:
		return sysenc.ComplexityAlphanumeric
	case cadestrov1.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_COMPLEX:
		return sysenc.ComplexityComplex
	default:
		return sysenc.ComplexityNone
	}
}
