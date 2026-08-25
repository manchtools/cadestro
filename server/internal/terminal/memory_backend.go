package terminal

import (
	"context"
	"sync"
	"time"
)

type MemoryBackend struct {
	mu     sync.Mutex
	now    func() time.Time
	values map[string]memoryEntry
}

type memoryEntry struct {
	payload   []byte
	expiresAt time.Time
}

func NewMemoryBackend(now func() time.Time) *MemoryBackend {
	if now == nil {
		now = time.Now
	}
	return &MemoryBackend{
		now:    now,
		values: make(map[string]memoryEntry),
	}
}

func (b *MemoryBackend) Set(ctx context.Context, sessionID string, payload []byte, ttl time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.values[sessionID] = memoryEntry{
		payload:   append([]byte(nil), payload...),
		expiresAt: b.now().Add(ttl),
	}
	return nil
}

func (b *MemoryBackend) Get(ctx context.Context, sessionID string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.values[sessionID]
	if !ok {
		return nil, ErrTokenNotFound
	}
	if !b.now().Before(entry.expiresAt) {
		delete(b.values, sessionID)
		return nil, ErrTokenNotFound
	}
	return append([]byte(nil), entry.payload...), nil
}

func (b *MemoryBackend) Delete(ctx context.Context, sessionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.values, sessionID)
	return nil
}

func (b *MemoryBackend) GetAndDelete(ctx context.Context, sessionID string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.values[sessionID]
	if !ok {
		return nil, ErrTokenNotFound
	}
	if !b.now().Before(entry.expiresAt) {
		delete(b.values, sessionID)
		return nil, ErrTokenNotFound
	}
	delete(b.values, sessionID)
	return append([]byte(nil), entry.payload...), nil
}
