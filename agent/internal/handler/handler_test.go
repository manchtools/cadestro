package handler

import (
	"context"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestResetConnectionWaitsForNextWelcome(t *testing.T) {
	h := NewHandler()
	if err := h.OnWelcome(context.Background(), &pb.Welcome{}); err != nil {
		t.Fatal(err)
	}
	h.ResetConnection()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := h.WaitConnected(ctx); err == nil {
		t.Fatal("WaitConnected returned before the next welcome")
	}

	if err := h.OnWelcome(context.Background(), &pb.Welcome{}); err != nil {
		t.Fatal(err)
	}
	if err := h.WaitConnected(context.Background()); err != nil {
		t.Fatal(err)
	}
}
