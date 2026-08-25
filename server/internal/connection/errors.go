package connection

import "errors"

var (
	ErrAgentNotConnected = errors.New("agent not connected")

	ErrSendTimeout = errors.New("timed out sending to agent")
)
