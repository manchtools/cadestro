package devicegroup_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/testdb"
)

func stringPtr(value string) *string { return &value }

func seedBareDevice(t *testing.T, raw *testdb.DB, deviceID string) {
	t.Helper()
	_, err := raw.Exec(context.Background(),
		`INSERT INTO devices (id, hostname) VALUES ($1, $2)`, deviceID, "host-"+deviceID)
	require.NoError(t, err)
}

func TestCreateDeviceGroup_TrueDynamicQueryMatchesAllDevices(t *testing.T) {
	h, raw := newScopeFixture(t)
	ctx := context.Background()

	deviceA, deviceB := newID(), newID()
	seedBareDevice(t, raw, deviceA)
	seedBareDevice(t, raw, deviceB)

	creator := &auth.UserContext{
		ID: newID(), Kind: auth.PrincipalUser,
		Permissions: []string{"CreateDynamicDeviceGroup", "EvaluateDynamicGroup"},
	}
	callerCtx := auth.WithUser(ctx, creator)

	created, err := h.CreateDeviceGroup(callerCtx, connect.NewRequest(&cadestrov1.CreateDeviceGroupRequest{
		Name: "everything", DynamicQuery: stringPtr("true"),
	}))
	require.NoError(t, err)
	require.NotNil(t, created.Msg.Group)
	assert.Equal(t, "true", created.Msg.Group.GetDynamicQuery())

	evaluated, err := h.EvaluateDynamicGroup(callerCtx, connect.NewRequest(&cadestrov1.EvaluateDynamicGroupRequest{
		Id: &cadestrov1.DeviceGroupId{Value: created.Msg.Group.GetId().GetValue()},
	}))
	require.NoError(t, err)
	assert.EqualValues(t, 2, evaluated.Msg.DevicesAdded)

	var members int
	require.NoError(t, raw.QueryRow(ctx,
		`SELECT count(*) FROM device_group_members WHERE group_id = $1`,
		created.Msg.Group.GetId().GetValue()).Scan(&members))
	assert.Equal(t, 2, members)
}

func TestCreateDeviceGroup_MalformedDynamicQueryStaysRejected(t *testing.T) {
	h, _ := newScopeFixture(t)
	creator := &auth.UserContext{
		ID: newID(), Kind: auth.PrincipalUser,
		Permissions: []string{"CreateDynamicDeviceGroup"},
	}
	_, err := h.CreateDeviceGroup(auth.WithUser(context.Background(), creator),
		connect.NewRequest(&cadestrov1.CreateDeviceGroupRequest{
			Name: "broken", DynamicQuery: stringPtr(`device.labels["env"] ==`),
		}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
