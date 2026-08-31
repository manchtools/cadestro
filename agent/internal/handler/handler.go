package handler

import (
	"context"
	"sync"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type Handler struct {
	mu        sync.Mutex
	connected chan struct{}
	ready     bool
}

func NewHandler() *Handler {
	return &Handler{connected: make(chan struct{})}
}

func (h *Handler) OnWelcome(context.Context, *pb.Welcome) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.ready {
		close(h.connected)
		h.ready = true
	}
	return nil
}

func (h *Handler) WaitConnected(ctx context.Context) error {
	h.mu.Lock()
	connected := h.connected
	h.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-connected:
		return nil
	}
}

func (h *Handler) ResetConnection() {
	h.mu.Lock()
	h.connected = make(chan struct{})
	h.ready = false
	h.mu.Unlock()
}
