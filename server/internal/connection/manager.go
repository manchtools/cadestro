package connection

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"connectrpc.com/connect"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type Agent struct {
	DeviceID    string
	Hostname    string
	Version     string
	ConnectedAt time.Time
	LastSeen    time.Time
	Stream      *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]
	sendMu      sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc

	write func(*cadestrov1.ServerMessage) error

	setWriteDeadline func(time.Time) error

	now func() time.Time
}

var SendTimeout = 10 * time.Second

func (a *Agent) Send(msg *cadestrov1.ServerMessage) error {

	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	select {
	case <-a.ctx.Done():
		return ErrAgentNotConnected
	default:
	}
	if a.write == nil {

		return ErrAgentNotConnected
	}

	if a.setWriteDeadline != nil && a.now != nil {
		if err := a.setWriteDeadline(a.now().Add(SendTimeout)); err != nil {

			a.setWriteDeadline = nil
		} else {

			defer func() { _ = a.setWriteDeadline(time.Time{}) }()
		}
	}

	err := a.write(msg)
	if err != nil && errors.Is(err, os.ErrDeadlineExceeded) {

		a.cancel()
		return ErrSendTimeout
	}
	return err
}

func (a *Agent) SetWriteDeadlineFunc(fn func(time.Time) error) {
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	a.setWriteDeadline = fn
}

func (a *Agent) WaitForInFlightSend() {
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
}

func (a *Agent) Close() {
	a.cancel()
}

func (a *Agent) Done() <-chan struct{} {
	return a.ctx.Done()
}

func (a *Agent) Terminated() bool {
	return a.ctx.Err() != nil
}

type Manager struct {
	now    func() time.Time
	mu     sync.RWMutex
	agents map[string]*Agent
}

func NewManager() *Manager {
	return &Manager{
		now:    time.Now,
		agents: make(map[string]*Agent),
	}
}

func (m *Manager) Register(parentCtx context.Context, deviceID, hostname, version string, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) *Agent {
	ctx, cancel := context.WithCancel(parentCtx)

	agent := &Agent{
		DeviceID:    deviceID,
		Hostname:    hostname,
		Version:     version,
		ConnectedAt: m.now(),
		LastSeen:    m.now(),
		Stream:      stream,
		ctx:         ctx,
		cancel:      cancel,
	}
	if stream != nil {
		agent.write = stream.Send
	}
	agent.now = m.now

	m.mu.Lock()

	if existing, ok := m.agents[deviceID]; ok {
		existing.Close()
	}
	m.agents[deviceID] = agent
	m.mu.Unlock()

	return agent
}

func (m *Manager) Unregister(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, ok := m.agents[deviceID]; ok {
		agent.Close()
		delete(m.agents, deviceID)
	}
}

func (m *Manager) UnregisterIfCurrent(deviceID string, agent *Agent) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, ok := m.agents[deviceID]
	if !ok || current != agent {
		return false
	}
	current.Close()
	delete(m.agents, deviceID)
	return true
}

func (m *Manager) Get(deviceID string) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, ok := m.agents[deviceID]
	return agent, ok
}

func (m *Manager) UpdateLastSeen(deviceID string) {
	m.mu.Lock()
	agent, ok := m.agents[deviceID]
	if ok {
		agent.LastSeen = m.now()
	}
	m.mu.Unlock()
}

func (m *Manager) LastSeenSnapshot() map[string]time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := make(map[string]time.Time, len(m.agents))
	for id, agent := range m.agents {
		snapshot[id] = agent.LastSeen
	}
	return snapshot
}

func (m *Manager) Send(deviceID string, msg *cadestrov1.ServerMessage) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, ok := m.agents[deviceID]

	if !ok {
		return ErrAgentNotConnected
	}

	return agent.Send(msg)
}

func (m *Manager) Broadcast(msg *cadestrov1.ServerMessage) {
	m.mu.RLock()
	agents := make([]*Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	m.mu.RUnlock()

	for _, agent := range agents {
		if err := agent.Send(msg); err != nil {
			slog.Warn("broadcast send failed", "device_id", agent.DeviceID, "error", err)
		}
	}
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.agents)
}

func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.agents))
	for id := range m.agents {
		ids = append(ids, id)
	}
	return ids
}

func (m *Manager) IsConnected(deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.agents[deviceID]
	return ok
}

func (m *Manager) Context(deviceID string) (context.Context, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if agent, ok := m.agents[deviceID]; ok {
		return agent.ctx, true
	}
	return nil, false
}
