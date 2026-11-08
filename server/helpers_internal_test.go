package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	hertz "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/stretchr/testify/assert"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"board", state.ErrBoardNotFound, http.StatusNotFound, "BOARD_NOT_FOUND"},
		{"device", state.ErrDeviceNotFound, http.StatusNotFound, "DEVICE_NOT_FOUND"},
		{"pin", state.ErrPinNotFound, http.StatusNotFound, "PIN_NOT_FOUND"},
		{"invalid id", state.ErrInvalidID, http.StatusBadRequest, "INVALID_IDENTIFIER"},
		{"invalid mode", state.ErrInvalidPinMode, http.StatusUnprocessableEntity, "INVALID_PIN_MODE"},
		{"read only", state.ErrReadOnly, http.StatusUnprocessableEntity, "READ_ONLY"},
		{"invalid value", state.ErrInvalidValue, http.StatusBadRequest, "INVALID_VALUE"},
		{"client cap", state.ErrClientCap, http.StatusTooManyRequests, "TOO_MANY_CLIENTS"},
		{"wrapped", fmt.Errorf("wrapped: %w", state.ErrDeviceNotFound), http.StatusNotFound, "DEVICE_NOT_FOUND"},
		{"generic", errors.New("something else"), http.StatusInternalServerError, "INTERNAL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code := classifyError(tt.err)
			assert.Equal(t, tt.status, status)
			assert.Equal(t, tt.code, code)
		})
	}
}

func TestParseEventID(t *testing.T) {
	tests := []struct {
		in   string
		want uint64
	}{
		{"", 0},
		{"0", 0},
		{"15", 15},
		{" 42 ", 42},
		{"abc", 0},
		{"-1", 0},
		{"99999999999999999999999999", 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, parseEventID(tt.in), "parseEventID(%q)", tt.in)
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"Bearer ", ""},
		{"Bearer", ""},
		{"Bearer token", "token"},
		{"bearer token", "token"},
		{"Basic dXNlcg==", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, bearerToken(tt.in), "bearerToken(%q)", tt.in)
	}
}

func TestConstantTimeEqual(t *testing.T) {
	assert.True(t, constantTimeEqual("secret", "secret"))
	assert.False(t, constantTimeEqual("secret", "other"))
	assert.False(t, constantTimeEqual("", "secret"))
	assert.False(t, constantTimeEqual("secret", ""))
}

func TestHealthAndAPIPath(t *testing.T) {
	assert.True(t, isHealthPath("/api/v1/health"))
	assert.False(t, isHealthPath("/api/v1/snapshot"))
	assert.False(t, isHealthPath("/api/v1/health/extra"))

	assert.True(t, isAPIPath("/api"))
	assert.True(t, isAPIPath("/api/v1/events"))
	assert.False(t, isAPIPath("/boards/dfp"))
	assert.False(t, isAPIPath("/apiary"))
}

func TestRequiresAuth(t *testing.T) {
	for _, path := range []string{
		"/api", "/api/v1/snapshot", "/api/v1/events", "/api/v1/health",
		"/arest/dfp", "/arest/dfp/", "/arest/dfp/digital/3/1",
		"/v1/devices/dfp", "/v1/devices/dfp/digitalread",
	} {
		assert.True(t, requiresAuth(path), "requiresAuth(%q) should be true", path)
	}
	for _, path := range []string{
		"/", "/index.html", "/simulator", "/_nuxt/app.js", "/boards/dfp", "/apiary",
	} {
		assert.False(t, requiresAuth(path), "requiresAuth(%q) should be false", path)
	}
}

func TestRecoveryMiddlewareHidesPanicValue(t *testing.T) {
	s := &Server{}
	h := hertz.New()
	h.Use(s.recoveryMiddleware)
	h.GET("/panic", func(_ context.Context, _ *app.RequestContext) {
		panic("super-secret-internal-value")
	})

	response := ut.PerformRequest(h.Engine, http.MethodGet, "/panic", nil)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "super-secret-internal-value")
	assert.Contains(t, response.Body.String(), "internal server error")
}
