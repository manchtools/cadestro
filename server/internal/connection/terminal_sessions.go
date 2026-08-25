package connection

import (
	"sync"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type TerminalSession struct {
	SessionID string
	DeviceID  string
	UserID    string
	TtyUser   string
	Cols      uint32
	Rows      uint32
	StartedAt time.Time

	OutputCh chan *cadestrov1.AgentMessage

	mu             sync.Mutex
	lastActivityAt time.Time
	now            func() time.Time
}

func NewTerminalSession(sessionID, deviceID, userID, ttyUser string, cols, rows uint32) *TerminalSession {
	clock := time.Now
	startedAt := clock()
	return &TerminalSession{
		SessionID:      sessionID,
		DeviceID:       deviceID,
		UserID:         userID,
		TtyUser:        ttyUser,
		Cols:           cols,
		Rows:           rows,
		StartedAt:      startedAt,
		lastActivityAt: startedAt,
		OutputCh:       make(chan *cadestrov1.AgentMessage, 64),
		now:            clock,
	}
}

func (s *TerminalSession) Touch() {
	s.mu.Lock()
	s.lastActivityAt = s.now()
	s.mu.Unlock()
}

func (s *TerminalSession) LastActivity() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastActivityAt
}

type TerminalSessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*TerminalSession
}

func NewTerminalSessionRegistry() *TerminalSessionRegistry {
	return &TerminalSessionRegistry{
		sessions: make(map[string]*TerminalSession),
	}
}

func (r *TerminalSessionRegistry) Register(s *TerminalSession) {
	r.mu.Lock()
	if old, exists := r.sessions[s.SessionID]; exists {
		close(old.OutputCh)
	}
	r.sessions[s.SessionID] = s
	r.mu.Unlock()
}

func (r *TerminalSessionRegistry) Unregister(sessionID string) {
	r.mu.Lock()
	if s, ok := r.sessions[sessionID]; ok {
		close(s.OutputCh)
		delete(r.sessions, sessionID)
	}
	r.mu.Unlock()
}

func (r *TerminalSessionRegistry) Get(sessionID string) *TerminalSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[sessionID]
}

func (r *TerminalSessionRegistry) RouteAgentMessage(sessionID string, msg *cadestrov1.AgentMessage) bool {

	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[sessionID]
	if !ok {
		return false
	}
	select {
	case s.OutputCh <- msg:
	default:

	}
	return true
}

func (r *TerminalSessionRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}

func (r *TerminalSessionRegistry) List() []*TerminalSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*TerminalSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}
