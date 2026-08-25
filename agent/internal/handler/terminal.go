package handler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	sdk "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
	"github.com/manchtools/cadestro/sdk/sys/terminal"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

var _ sdk.TerminalHandler = (*Handler)(nil)

var (
	termUserMgr   = mustTermUserManager()
	termFSMgr     = mustTermFSManager()
	sysuserModify = termUserMgr.Modify
	sysuserGet    = termUserMgr.Get

	terminalSetupTimeout = 30 * time.Second

	terminalCleanupTimeout = 5 * time.Second
)

func terminalCleanupContext(requestCtx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(requestCtx), terminalCleanupTimeout)
}

func mustTermUserManager() sysuser.Manager {
	m, err := sysuser.New(sysuser.ShadowUtils, handlerRunner)
	if err != nil {
		panic("handler: user manager must construct: " + err.Error())
	}
	return m
}

func mustTermFSManager() sysfs.Manager {
	m, err := sysfs.New(handlerRunner)
	if err != nil {
		panic("handler: fs manager must construct: " + err.Error())
	}
	return m
}

const (
	defaultTerminalLimit       = 3
	defaultTerminalIdleTimeout = 30 * time.Minute
	terminalSweepInterval      = 30 * time.Second
	terminalReadChunkBytes     = 32 * 1024

	terminalActivatedShell   = "/bin/bash"
	terminalDeactivatedShell = "/usr/sbin/nologin"

	maxTerminalDimension = 65535
)

func validateDims(cols, rows uint32) error {
	if cols == 0 || cols > maxTerminalDimension {
		return fmt.Errorf("invalid terminal dimensions: cols=%d (must be 1..%d)", cols, maxTerminalDimension)
	}
	if rows == 0 || rows > maxTerminalDimension {
		return fmt.Errorf("invalid terminal dimensions: rows=%d (must be 1..%d)", rows, maxTerminalDimension)
	}
	return nil
}

type TerminalSender interface {
	SendTerminalOutput(ctx context.Context, out *pb.TerminalOutput) error
	SendTerminalStateChange(ctx context.Context, change *pb.TerminalStateChange) error
}

type sessionState int

const (
	sessionStateStarting sessionState = iota

	sessionStateActive

	sessionStateStopping
)

type terminalSession struct {
	id      string
	ttyUser string

	sender TerminalSender

	mu           sync.Mutex
	state        sessionState
	session      *terminal.Session
	tempHome     string
	cancel       context.CancelFunc
	lastActivity time.Time

	now func() time.Time
}

func (ts *terminalSession) touch() {
	ts.mu.Lock()
	ts.lastActivity = ts.now()
	ts.mu.Unlock()
}

func (ts *terminalSession) idleSince() time.Time {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.lastActivity
}

func (ts *terminalSession) isStopping() bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.state == sessionStateStopping
}

func (h *Handler) SetTerminalSender(sender TerminalSender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.terminalSender = sender
	if h.terminals == nil {
		h.terminals = make(map[string]*terminalSession)
	}
	if h.terminalLimit == 0 {
		h.terminalLimit = defaultTerminalLimit
	}
	if h.terminalIdleTimeout == 0 {
		h.terminalIdleTimeout = defaultTerminalIdleTimeout
	}
	if !h.terminalSweeperStarted {
		h.terminalSweeperStarted = true
		h.terminalSweeperStop = make(chan struct{})
		stopCh := h.terminalSweeperStop
		go h.terminalSweepLoop(stopCh)
	}
}

func (h *Handler) StopTerminalSweeper() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.terminalSweeperStarted {
		return
	}
	if h.terminalSweeperStop == nil {
		return
	}

	close(h.terminalSweeperStop)
	h.terminalSweeperStop = nil
	h.terminalSweeperStarted = false
}

func (h *Handler) snapshotTerminalSender() TerminalSender {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.terminalSender
}

func (h *Handler) OnTerminalStart(ctx context.Context, req *pb.TerminalStart) error {
	logger := h.logger.With("session_id", req.GetSessionId().GetValue(), "tty_user", req.TtyUser)
	logger.Info("opening terminal session")

	sender := h.snapshotTerminalSender()
	if sender == nil {

		logger.Error("terminal sender not configured; dropping start request")
		return nil
	}

	if !sysuser.IsValidName(req.TtyUser) || !strings.HasPrefix(req.TtyUser, terminal.TTYUsernamePrefix) {
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), "invalid tty username")
		return nil
	}

	if h.store == nil {
		logger.Warn("terminal start rejected: no store wired for tty gate")
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), "terminal sessions are disabled on this device")
		return nil
	}
	enabled, err := h.store.IsTTYEnabled(ctx)
	if err != nil {
		logger.Warn("failed to read tty toggle state; refusing session", "error", err)
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), "terminal sessions are disabled on this device")
		return nil
	}
	if !enabled {
		logger.Info("terminal start rejected: tty disabled on device")
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), "terminal sessions are disabled on this device")
		return nil
	}

	if err := validateDims(req.Cols, req.Rows); err != nil {
		logger.Warn("terminal start rejected: bad dimensions", "cols", req.Cols, "rows", req.Rows)
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), err.Error())
		return nil
	}

	if _, err := ulid.Parse(req.GetSessionId().GetValue()); err != nil {
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), "invalid session id")
		return nil
	}

	info, err := sysuserGet(ctx, req.TtyUser)
	if err != nil {
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), fmt.Sprintf("tty user %q not provisioned: %v", req.TtyUser, err))
		return nil
	}
	if info.Locked {
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), fmt.Sprintf("tty user %q is disabled", req.TtyUser))
		return nil
	}

	sessionCtx, cancel := context.WithCancel(context.Background())

	setupCtx, setupCancel := context.WithTimeout(sessionCtx, terminalSetupTimeout)
	defer setupCancel()
	ts := &terminalSession{
		id:      req.GetSessionId().GetValue(),
		ttyUser: req.TtyUser,
		sender:  sender,
		state:   sessionStateStarting,
		cancel:  cancel,
		now:     time.Now,
	}
	ts.touch()

	h.mu.Lock()
	if h.terminals == nil {
		h.terminals = make(map[string]*terminalSession)
	}
	if _, exists := h.terminals[req.GetSessionId().GetValue()]; exists {
		h.mu.Unlock()
		cancel()
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), "session already exists")
		return nil
	}
	limit := h.terminalLimit
	if limit == 0 {
		limit = defaultTerminalLimit
	}
	if len(h.terminals) >= limit {
		h.mu.Unlock()
		cancel()
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), fmt.Sprintf("device terminal session limit reached (%d)", limit))
		return nil
	}
	h.terminals[req.GetSessionId().GetValue()] = ts
	h.mu.Unlock()

	var (
		shellActivated bool
		tempHomeDir    string
	)

	cleanup := func() {
		if tempHomeDir != "" {
			if err := os.RemoveAll(tempHomeDir); err != nil {
				logger.Warn("failed to remove terminal temp home", "path", tempHomeDir, "error", err)
			}
		}
		if shellActivated {

			if !h.anySessionForUserExcept(req.TtyUser, req.GetSessionId().GetValue()) {
				h.deactivateShell(ctx, req.TtyUser)
			}
		}
		h.removeTerminal(req.GetSessionId().GetValue())
	}

	abortFail := func(reason string) {
		cleanup()
		h.failTerminalStart(ctx, sender, req.GetSessionId().GetValue(), reason)
	}

	abortStopped := func() {
		logger.Info("terminal start aborted by concurrent stop")
		cleanup()
	}

	if ts.isStopping() {
		abortStopped()
		return nil
	}
	if err := sysuserModify(setupCtx, req.TtyUser, sysuser.ModifyOptions{Shell: terminalActivatedShell}); err != nil {
		abortFail(fmt.Sprintf("activate shell: %v", err))
		return nil
	}
	shellActivated = true

	if ts.isStopping() {
		abortStopped()
		return nil
	}
	tempHome := filepath.Join("/tmp", req.TtyUser+"."+req.GetSessionId().GetValue())
	if err := os.Mkdir(tempHome, 0o700); err != nil {
		abortFail(fmt.Sprintf("create temp home: %v", err))
		return nil
	}
	tempHomeDir = tempHome

	if info, err := os.Lstat(tempHome); err != nil || !info.Mode().IsDir() {
		abortFail("temp home is not a regular directory")
		return nil
	}
	if err := termFSMgr.SetOwnershipRecursive(setupCtx, tempHome, req.TtyUser, req.TtyUser); err != nil {
		abortFail(fmt.Sprintf("chown temp home: %v", err))
		return nil
	}

	if ts.isStopping() {
		abortStopped()
		return nil
	}

	cols := uint16(req.Cols)
	rows := uint16(req.Rows)
	cfg := terminal.SessionConfig{
		User:    req.TtyUser,
		Shell:   terminalActivatedShell,
		Cols:    cols,
		Rows:    rows,
		WorkDir: tempHome,
		Env:     []string{"HOME=" + tempHome, "USER=" + req.TtyUser, "LOGNAME=" + req.TtyUser},
	}

	tm, err := terminal.New()
	if err != nil {
		abortFail(fmt.Sprintf("build terminal manager: %v", err))
		return nil
	}
	sess, err := tm.Open(setupCtx, cfg)
	if err != nil {
		abortFail(fmt.Sprintf("allocate pty: %v", err))
		return nil
	}

	ts.mu.Lock()
	if ts.state == sessionStateStopping {
		ts.mu.Unlock()
		_ = sess.Close()
		abortStopped()
		return nil
	}
	ts.session = sess
	ts.tempHome = tempHomeDir
	ts.state = sessionStateActive
	ts.touchLocked()
	ts.mu.Unlock()

	if err := sender.SendTerminalStateChange(ctx, &pb.TerminalStateChange{
		SessionId: req.GetSessionId(),
		State:     pb.TerminalSessionState_TERMINAL_SESSION_STATE_STARTED,
	}); err != nil {
		logger.Warn("failed to send STARTED state change; aborting session", "error", err)
		cleanupCtx, cancelCleanup := terminalCleanupContext(ctx)
		h.closeTerminal(cleanupCtx, req.GetSessionId().GetValue(), "send started failed")
		cancelCleanup()
		return nil
	}

	go h.pumpTerminalOutput(sessionCtx, ts)
	return nil
}

func (ts *terminalSession) touchLocked() {
	ts.lastActivity = ts.now()
}

func (h *Handler) OnTerminalInput(ctx context.Context, req *pb.TerminalInput) error {
	ts := h.lookupTerminal(req.GetSessionId().GetValue())
	if ts == nil {
		h.logger.Debug("terminal input for unknown session", "session_id", req.GetSessionId().GetValue())
		return nil
	}

	ts.mu.Lock()
	sess := ts.session
	ts.mu.Unlock()
	if sess == nil {
		h.logger.Debug("terminal input for not-yet-active session", "session_id", req.GetSessionId().GetValue())
		return nil
	}
	if _, err := sess.Write(req.Data); err != nil {
		h.logger.Warn("terminal input write failed", "session_id", req.GetSessionId().GetValue(), "error", err)

		return nil
	}
	ts.touch()
	return nil
}

func (h *Handler) OnTerminalResize(ctx context.Context, req *pb.TerminalResize) error {

	if err := validateDims(req.Cols, req.Rows); err != nil {
		h.logger.Warn("ignoring terminal resize with bad dimensions",
			"session_id", req.GetSessionId().GetValue(), "cols", req.Cols, "rows", req.Rows)
		return nil
	}
	ts := h.lookupTerminal(req.GetSessionId().GetValue())
	if ts == nil {
		h.logger.Debug("terminal resize for unknown session", "session_id", req.GetSessionId().GetValue())
		return nil
	}
	ts.mu.Lock()
	sess := ts.session
	ts.mu.Unlock()
	if sess == nil {
		h.logger.Debug("terminal resize for not-yet-active session", "session_id", req.GetSessionId().GetValue())
		return nil
	}
	if err := sess.Resize(uint16(req.Cols), uint16(req.Rows)); err != nil {
		h.logger.Warn("terminal resize failed", "session_id", req.GetSessionId().GetValue(), "error", err)
	}
	return nil
}

func (h *Handler) OnTerminalStop(ctx context.Context, req *pb.TerminalStop) error {
	if req.Reason != "" {
		h.logger.Info("stopping terminal session", "session_id", req.GetSessionId().GetValue(), "reason", req.Reason)
	} else {
		h.logger.Info("stopping terminal session", "session_id", req.GetSessionId().GetValue())
	}
	h.closeTerminal(ctx, req.GetSessionId().GetValue(), req.Reason)
	return nil
}

func (h *Handler) pumpTerminalOutput(sessionCtx context.Context, ts *terminalSession) {
	defer func() {

		exitCode, _ := ts.session.Wait()
		state := &pb.TerminalStateChange{
			SessionId: &pb.SessionId{Value: ts.id},
			State:     pb.TerminalSessionState_TERMINAL_SESSION_STATE_EXITED,
			ExitCode:  int32(exitCode),
		}

		sendCtx, cancel := terminalCleanupContext(sessionCtx)
		err := ts.sender.SendTerminalStateChange(sendCtx, state)
		cancel()
		if err != nil {
			h.logger.Warn("failed to send EXITED state change",
				"session_id", ts.id, "error", err)
		}

		cleanupCtx, cancelCleanup := terminalCleanupContext(sessionCtx)
		h.closeTerminal(cleanupCtx, ts.id, "")
		cancelCleanup()
	}()

	buf := make([]byte, terminalReadChunkBytes)
	for {
		select {
		case <-sessionCtx.Done():
			return
		default:
		}

		n, err := ts.session.Read(buf)
		if n > 0 {
			ts.touch()
			out := &pb.TerminalOutput{
				SessionId: &pb.SessionId{Value: ts.id},
				Data:      append([]byte(nil), buf[:n]...),
			}
			if sendErr := ts.sender.SendTerminalOutput(sessionCtx, out); sendErr != nil {
				h.logger.Warn("failed to send terminal output; tearing down session",
					"session_id", ts.id, "error", sendErr)

				return
			}
		}
		if err != nil {

			return
		}
	}
}

func (h *Handler) failTerminalStart(ctx context.Context, sender TerminalSender, sessionID, msg string) {
	h.logger.Warn("terminal session start failed", "session_id", sessionID, "error", msg)
	if sender == nil {
		return
	}
	change := &pb.TerminalStateChange{
		SessionId: &pb.SessionId{Value: sessionID},
		State:     pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR,
		Error:     msg,
	}
	if err := sender.SendTerminalStateChange(ctx, change); err != nil {
		h.logger.Warn("failed to send ERROR state change",
			"session_id", sessionID, "error", err)
	}
}

func (h *Handler) closeTerminal(ctx context.Context, sessionID, reason string) {

	h.logger.Debug("closeTerminal entered",
		"session_id", sessionID, "reason", reason)

	h.mu.Lock()
	ts, ok := h.terminals[sessionID]
	h.mu.Unlock()
	if !ok || ts == nil {
		return
	}

	ts.mu.Lock()
	if ts.state == sessionStateStopping {

		ts.mu.Unlock()
		return
	}
	wasStarting := ts.state == sessionStateStarting
	ts.state = sessionStateStopping
	if ts.cancel != nil {
		ts.cancel()
	}
	sess := ts.session
	tempHome := ts.tempHome
	ttyUser := ts.ttyUser
	ts.mu.Unlock()

	if wasStarting {

		return
	}

	h.mu.Lock()
	delete(h.terminals, sessionID)
	h.mu.Unlock()

	if sess != nil {
		if err := sess.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			h.logger.Warn("terminal session close error", "session_id", sessionID, "error", err)
		}
	}

	h.mu.Lock()
	stillActiveForUser := false
	for _, other := range h.terminals {
		if other != nil && other.ttyUser == ttyUser {
			stillActiveForUser = true
			break
		}
	}
	h.mu.Unlock()
	if !stillActiveForUser {
		h.deactivateShell(ctx, ttyUser)
	}

	if tempHome != "" {
		if err := os.RemoveAll(tempHome); err != nil {
			h.logger.Warn("failed to remove terminal temp home",
				"session_id", sessionID, "path", tempHome, "error", err)
		}
	}
}

func (h *Handler) CloseAllTerminals(ctx context.Context) {
	h.mu.Lock()
	ids := make([]string, 0, len(h.terminals))
	for id := range h.terminals {
		ids = append(ids, id)
	}
	h.mu.Unlock()

	for _, id := range ids {
		h.closeTerminal(ctx, id, "agent shutdown")
	}
}

func (h *Handler) anySessionForUserExcept(ttyUser, exceptSessionID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, other := range h.terminals {
		if id == exceptSessionID || other == nil {
			continue
		}
		if other.ttyUser == ttyUser {
			return true
		}
	}
	return false
}

func (h *Handler) deactivateShell(ctx context.Context, ttyUser string) {
	if err := sysuserModify(ctx, ttyUser, sysuser.ModifyOptions{Shell: terminalDeactivatedShell}); err != nil {
		h.logger.Warn("failed to revert tty user shell",
			"tty_user", ttyUser, "error", err)
	}
}

func (h *Handler) lookupTerminal(sessionID string) *terminalSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.terminals[sessionID]
}

func (h *Handler) removeTerminal(sessionID string) {
	h.mu.Lock()
	ts := h.terminals[sessionID]
	delete(h.terminals, sessionID)
	h.mu.Unlock()

	if ts != nil && ts.cancel != nil {
		ts.cancel()
	}
}

func (h *Handler) terminalSweepLoop(stopCh <-chan struct{}) {
	t := time.NewTicker(terminalSweepInterval)
	defer t.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-t.C:
			h.sweepIdleTerminals()
		}
	}
}

func (h *Handler) sweepIdleTerminals() {
	h.mu.Lock()
	timeout := h.terminalIdleTimeout
	if timeout == 0 {
		timeout = defaultTerminalIdleTimeout
	}
	cutoff := h.now().Add(-timeout)
	var idle []string
	for id, ts := range h.terminals {
		if ts == nil {
			continue
		}

		ts.mu.Lock()
		state := ts.state
		ts.mu.Unlock()
		if state != sessionStateActive {
			continue
		}
		if ts.idleSince().Before(cutoff) {
			idle = append(idle, id)
		}
	}
	h.mu.Unlock()

	for _, id := range idle {
		h.logger.Info("closing idle terminal session", "session_id", id, "timeout", timeout)
		cleanupCtx, cancelCleanup := terminalCleanupContext(context.Background())
		h.closeTerminal(cleanupCtx, id, "idle timeout")
		cancelCleanup()
	}
}
