package handler

import (
	"context"
	"testing"
	"time"
)

func TestRemoveTerminal_CancelsSessionContext(t *testing.T) {
	h := &Handler{}
	sessionCtx, cancel := context.WithCancel(context.Background())
	h.terminals = map[string]*terminalSession{
		"s1": {id: "s1", cancel: cancel, now: time.Now},
	}

	h.removeTerminal("s1")

	select {
	case <-sessionCtx.Done():
	default:
		t.Fatal("removeTerminal must cancel the session context — aborted starts leaked it")
	}
	if _, exists := h.terminals["s1"]; exists {
		t.Fatal("session must be removed from the registry")
	}

	h.removeTerminal("does-not-exist")
}
