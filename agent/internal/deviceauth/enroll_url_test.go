package deviceauth

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestEnroll_RejectsNonHTTPSServerURL(t *testing.T) {
	cases := []string{
		"http://control.example.com",
		"HTTP://control.example.com",
		"ftp://control.example.com",
		"control.example.com",
		"https:foo",
		"https:",
		"https://user:pass@host",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			credStore := credentials.NewStore(t.TempDir())
			h := NewEnrollHandler("h", "dev", credStore, slog.Default(), nil)

			resp, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
				ServerUrl: u, Token: "some-token", CaFingerprintPin: testCAPin,
			}))
			require.NoError(t, err)
			assert.False(t, resp.Msg.Success)
			assert.Contains(t, resp.Msg.Error, "https")
			assert.False(t, credStore.Exists(), "no credentials must be saved on a rejected URL")
		})
	}
}

func TestEnroll_PerFieldRequired(t *testing.T) {
	cases := []struct {
		name       string
		url, token string
	}{
		{"token absent", "https://control.example.com", ""},
		{"server_url absent", "", "tok"},
		{"both absent", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			credStore := credentials.NewStore(t.TempDir())
			h := NewEnrollHandler("h", "dev", credStore, slog.Default(), nil)

			resp, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
				ServerUrl: tc.url, Token: tc.token, CaFingerprintPin: testCAPin,
			}))
			require.NoError(t, err)
			assert.False(t, resp.Msg.Success)
			assert.Contains(t, resp.Msg.Error, "required")
			assert.False(t, credStore.Exists())
		})
	}
}
