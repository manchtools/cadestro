package agentstream

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
)

func TestFrameNotAuthorizedClassifiesRetainedSecurityErrors(t *testing.T) {
	assert.True(t, frameNotAuthorized(errForeignTerminalSession))
	assert.True(t, frameNotAuthorized(connect.NewError(connect.CodeUnauthenticated, errors.New("no identity"))))
	assert.True(t, frameNotAuthorized(connect.NewError(connect.CodePermissionDenied, errors.New("denied"))))
	assert.False(t, frameNotAuthorized(errors.New("invalid result")))
	assert.False(t, frameNotAuthorized(connect.NewError(connect.CodeNotFound, errors.New("gone"))))
}
