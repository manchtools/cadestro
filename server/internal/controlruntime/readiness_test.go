package controlruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadinessHandlerFailsClosed(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	health := health("test")

	ok := httptest.NewRecorder()
	readinessHandler(func(context.Context) error { return nil }, health)(ok, request)
	assert.Equal(t, http.StatusOK, ok.Code)
	assert.Equal(t, "application/json", ok.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"version":"test"}`, ok.Body.String())

	unavailable := httptest.NewRecorder()
	readinessHandler(func(context.Context) error { return errors.New("unavailable") }, health)(unavailable, request)
	assert.Equal(t, http.StatusServiceUnavailable, unavailable.Code)
	assert.NotContains(t, unavailable.Body.String(), "unavailable", "readiness must not expose internal errors")
}

func TestHealthHandlerReturnsVersionJSON(t *testing.T) {
	response := httptest.NewRecorder()
	health("v1.2.3")(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"version":"v1.2.3"}`, response.Body.String())
}
