package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/sdk/sys/desktop"
)

func TestRunAsUser_EmptyNameRejected(t *testing.T) {
	_, err := runAsUser(context.Background(),
		desktop.Session{Username: "alice", Home: "/home/alice"}, nil, "", "", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errEmptyName), "want errEmptyName, got %v", err)
}

func TestRunAsUser_EmptyUsernameRejected(t *testing.T) {
	_, err := runAsUser(context.Background(),
		desktop.Session{Username: ""}, nil, "", "echo", []string{"hi"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errEmptyUsername), "want errEmptyUsername, got %v", err)
}
