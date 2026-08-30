package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
)

func TestOIDCTransactionCookieAttributes(t *testing.T) {
	cookie := oidcTransactionCookie("signed", authStateTTL)
	if cookie.Name != "__Host-cadestro-oidc" || cookie.Value != "signed" || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteNoneMode || cookie.MaxAge != 600 {
		t.Fatalf("transaction cookie = %+v", cookie)
	}
	cleared := oidcTransactionCookie("", -1)
	if cleared.Value != "" || cleared.MaxAge != -1 {
		t.Fatalf("cleared transaction cookie = %+v", cleared)
	}
}

func TestSSOCallbackRejectsMismatchedCookieState(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := auth.NewJWTManager(auth.JWTConfig{PrivateKey: privateKey})
	if err != nil {
		t.Fatal(err)
	}
	value, err := manager.SignOIDCTransaction("cookie-state", "provider", "nonce", "verifier", "https://web.example/callback", time.Now().Add(authStateTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := connect.NewRequest(&cadestrov1.SSOCallbackRequest{State: "request-state"})
	request.Header().Set("Cookie", (&http.Cookie{Name: oidcTransactionCookieName, Value: value}).String())
	service := &Service{jwt: manager}
	_, err = service.SSOCallback(context.Background(), request)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("SSOCallback() error = %v", err)
	}
}

type cookieClearingHandler struct {
	cadestrov1connect.UnimplementedControlServiceHandler
}

func (cookieClearingHandler) SSOCallback(ctx context.Context, _ *connect.Request[cadestrov1.SSOCallbackRequest]) (*connect.Response[cadestrov1.SSOCallbackResponse], error) {
	clearOIDCTransactionCookie(ctx)
	return nil, connect.NewError(connect.CodeInternal, errors.New("test failure"))
}

func TestClearOIDCTransactionCookieReachesErrorResponse(t *testing.T) {
	_, handler := cadestrov1connect.NewControlServiceHandler(cookieClearingHandler{})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := cadestrov1connect.NewControlServiceClient(http.DefaultClient, server.URL)
	ctx, callInfo := connect.NewClientContext(context.Background())
	_, err := client.SSOCallback(ctx, connect.NewRequest(&cadestrov1.SSOCallbackRequest{}))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("SSOCallback() error = %v", err)
	}
	if len(callInfo.ResponseHeader().Values("Set-Cookie")) != 1 {
		t.Fatalf("Set-Cookie = %v", callInfo.ResponseHeader().Values("Set-Cookie"))
	}
	responseCookies := (&http.Response{Header: callInfo.ResponseHeader()}).Cookies()
	if len(responseCookies) != 1 {
		t.Fatalf("response cookies = %v", responseCookies)
	}
	responseCookie := responseCookies[0]
	if responseCookie.Name != oidcTransactionCookieName || responseCookie.Value != "" || responseCookie.MaxAge != -1 {
		t.Fatalf("clearing response cookie = %+v", responseCookie)
	}
}
