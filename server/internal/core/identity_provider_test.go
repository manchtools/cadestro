package core

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/idp"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func TestDeleteIdentityProviderChecksRequestedProviderBeforeLastProviderGuard(t *testing.T) {
	service, ctx, _, _ := testService(t)
	providerID := "01K00000000000000000000301"
	_, err := service.store.Queries().CreateIdentityProvider(ctx, db.CreateIdentityProviderParams{
		ID: providerID, Name: "SSO", Slug: "sso", Enabled: true, ClientID: "client",
		IssuerUrl: "https://issuer.example", ScopesJson: idp.Scopes{},
	})
	require.NoError(t, err)
	_, err = service.DeleteIdentityProvider(ctx, connect.NewRequest(&cadestrov1.DeleteIdentityProviderRequest{
		Id: &cadestrov1.IdentityProviderId{Value: "01K00000000000000000000302"},
	}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = service.DeleteIdentityProvider(ctx, connect.NewRequest(&cadestrov1.DeleteIdentityProviderRequest{
		Id: &cadestrov1.IdentityProviderId{Value: providerID},
	}))
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}
