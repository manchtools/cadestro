package identity_test

import (
	"testing"

	"connectrpc.com/connect"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/stretchr/testify/assert"
)

func TestGetRole_RejectsMalformedIDBeforeAuthentication(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	_, err := f.client.GetRole(f.ctx(), connect.NewRequest(&cadestrov1.GetRoleRequest{Id: "not-a-ulid"}))

	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
