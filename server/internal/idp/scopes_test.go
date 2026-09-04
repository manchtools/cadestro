package idp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopesRoundTrip(t *testing.T) {
	want := Scopes{"openid", "profile", "email"}
	encoded, err := want.Value()
	require.NoError(t, err)
	for _, source := range []any{encoded, []byte(encoded.(string))} {
		var got Scopes
		require.NoError(t, got.Scan(source))
		require.Equal(t, want, got)
	}
}

func TestScopesRejectMalformedStorage(t *testing.T) {
	for _, source := range []any{"{", []byte("{}"), 1, nil} {
		var scopes Scopes
		require.Error(t, scopes.Scan(source))
	}
}
