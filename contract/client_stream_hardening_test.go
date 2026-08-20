package contract

import (
	"context"
	"strings"
	"testing"
	"time"

	pm "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func strictDispatchClient() *Client {
	c := NewClient("https://gw.invalid")
	c.requireWelcome = true
	return c
}

func TestDispatchRejectsInvalidServerEnvelope(t *testing.T) {
	for name, msg := range map[string]*pm.ServerMessage{
		"nil":                 nil,
		"bad id":              {Id: "not-a-ulid", Payload: &pm.ServerMessage_Welcome{Welcome: &pm.Welcome{}}},
		"nil payload":         {Id: NewULID()},
		"nil payload message": {Id: NewULID(), Payload: &pm.ServerMessage_Welcome{}},
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
	msg := &pm.ServerMessage{
		Id: NewULID(),
		Payload: &pm.ServerMessage_Query{Query: &pm.OSQuery{
			QueryId: validULID,
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
