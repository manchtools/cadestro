package contract

import (
	"context"
	"strings"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func strictDispatchClient() *Client {
	c := NewClient("https://gw.invalid")
	c.requireWelcome = true
	return c
}

func TestDispatchRejectsInvalidServerEnvelope(t *testing.T) {
	for name, msg := range map[string]*cadestrov1.ServerMessage{
		"nil":                 nil,
		"bad id":              {Id: "not-a-ulid", Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{}}},
		"nil payload":         {Id: NewULID()},
		"nil payload message": {Id: NewULID(), Payload: &cadestrov1.ServerMessage_Welcome{}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := strictDispatchClient().dispatchServerMessage(context.Background(), msg, fakeBareHandler{}); err == nil {
				t.Fatal("invalid ServerMessage was accepted")
			}
		})
	}
}

func TestDispatchRejectsBeforeWelcome(t *testing.T) {
	c := strictDispatchClient()
	msg := &cadestrov1.ServerMessage{
		Id: NewULID(),
		Payload: &cadestrov1.ServerMessage_Query{Query: &cadestrov1.OSQuery{
			QueryId: &cadestrov1.QueryId{Value: validULID},
			Table:   "processes",
		}},
	}
	err := c.dispatchServerMessage(context.Background(), msg, &recordingHandler{})
	if err == nil || !strings.Contains(err.Error(), "first server message must be Welcome") {
		t.Fatalf("pre-Welcome dispatch error = %v", err)
	}
}

func TestNormalizeHeartbeatInterval(t *testing.T) {
	if got := normalizeHeartbeatInterval(0); got != MinHeartbeatInterval {
		t.Fatalf("zero interval = %s, want %s", got, MinHeartbeatInterval)
	}
	if got := normalizeHeartbeatInterval(-time.Second); got != MinHeartbeatInterval {
		t.Fatalf("negative interval = %s, want %s", got, MinHeartbeatInterval)
	}
}
