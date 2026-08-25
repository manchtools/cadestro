package executor

import (
	"os/user"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

func TestHomeGroupForOwnership_ResolvesViaSDK(t *testing.T) {
	cur, err := user.Current()
	require.NoError(t, err)
	wantUID, err := strconv.Atoi(cur.Uid)
	require.NoError(t, err)
	wantPrimaryGID, err := strconv.Atoi(cur.Gid)
	require.NoError(t, err)

	resolve := func(p *pb.UserParams) (int, int, error) {
		return sysfs.ResolveOwnership(p.Username, homeGroupFor(p))
	}

	t.Run("numeric Gid is used as a literal GID (no name lookup)", func(t *testing.T) {

		uid, gid, err := resolve(&pb.UserParams{Username: cur.Username, Gid: 4242})
		require.NoError(t, err)
		assert.Equal(t, wantUID, uid)
		assert.Equal(t, 4242, gid)
	})

	t.Run("named PrimaryGroup is resolved via the group database", func(t *testing.T) {
		grp, err := user.LookupGroupId(cur.Gid)
		require.NoError(t, err)
		uid, gid, err := resolve(&pb.UserParams{Username: cur.Username, PrimaryGroup: grp.Name})
		require.NoError(t, err)
		assert.Equal(t, wantUID, uid)
		assert.Equal(t, wantPrimaryGID, gid)
	})

	t.Run("unknown user is an error, not a silent uid 0", func(t *testing.T) {
		_, _, err := resolve(&pb.UserParams{Username: "cadestro-definitely-no-such-user-xyz"})
		assert.Error(t, err, "a failed user lookup must error so .ssh is never chowned to root")
	})
}

func TestDesiredAccountLocked(t *testing.T) {
	cases := []struct {
		name string
		p    *pb.UserParams
		want bool
	}{

		{"no_password, not disabled -> unlocked", &pb.UserParams{NoPassword: true, Disabled: false}, false},
		{"no_password, disabled -> locked", &pb.UserParams{NoPassword: true, Disabled: true}, true},

		{"system_user, not disabled -> unlocked", &pb.UserParams{SystemUser: true, Disabled: false}, false},
		{"system_user, disabled -> locked", &pb.UserParams{SystemUser: true, Disabled: true}, true},

		{"normal enabled -> unlocked", &pb.UserParams{}, false},
		{"normal disabled -> locked", &pb.UserParams{Disabled: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, desiredAccountLocked(tc.p))
		})
	}
}

func TestDesiredAccountLocked_IsExactlyDisabled(t *testing.T) {
	for _, noPass := range []bool{false, true} {
		for _, sysUser := range []bool{false, true} {
			for _, disabled := range []bool{false, true} {
				p := &pb.UserParams{NoPassword: noPass, SystemUser: sysUser, Disabled: disabled}
				assert.Equal(t, disabled, desiredAccountLocked(p),
					"lock must track Disabled only: no_password=%v system_user=%v disabled=%v", noPass, sysUser, disabled)
			}
		}
	}

	assert.True(t, createUserSetsPassword(&pb.UserParams{}),
		"a plain account (no opt-outs) must get a password")
	assert.False(t, createUserSetsPassword(&pb.UserParams{NoPassword: true}), "no_password skips the password")
	assert.False(t, createUserSetsPassword(&pb.UserParams{SystemUser: true}), "system_user skips the password")
	assert.False(t, createUserSetsPassword(&pb.UserParams{Disabled: true}), "disabled skips the password")
}
