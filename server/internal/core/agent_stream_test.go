package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	connectvalidate "connectrpc.com/validate"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/stretchr/testify/require"
)

func TestAgentStreamPreservesValidationErrorDetails(t *testing.T) {
	path, handler := cadestrov1connect.NewAgentServiceHandler(&Service{}, connect.WithInterceptors(connectvalidate.NewInterceptor()))
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := cadestrov1connect.NewAgentServiceClient(server.Client(), server.URL)
	stream := client.Stream(ctx)
	require.NoError(t, stream.Send(&cadestrov1.AgentMessage{Id: &cadestrov1.MessageId{Value: "01K00000000000000000000078"}}))
	require.NoError(t, stream.CloseRequest())
	_, err := stream.Receive()
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	require.NotEmpty(t, connectErr.Details())
}
