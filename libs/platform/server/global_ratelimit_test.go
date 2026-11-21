package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGlobalRateLimit(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := GlobalRateLimit(0.001, 2)(ok)

	call := func(method, path, addr string) int {
		r := httptest.NewRequest(method, path, nil)
		r.RemoteAddr = addr
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr.Code
	}

	assert.Equal(t, http.StatusOK, call(http.MethodGet, "/api/v1/players", "1.1.1.1:1"))
	assert.Equal(t, http.StatusOK, call(http.MethodGet, "/api/v1/players", "1.1.1.1:1"))
	assert.Equal(t, http.StatusTooManyRequests, call(http.MethodGet, "/api/v1/players", "1.1.1.1:1"))

	t.Run("other addresses keep their own allowance", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, call(http.MethodGet, "/api/v1/players", "2.2.2.2:1"))
	})
	t.Run("probes and preflights are never limited", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, call(http.MethodGet, "/health", "1.1.1.1:1"))
		assert.Equal(t, http.StatusOK, call(http.MethodOptions, "/api/v1/players", "1.1.1.1:1"))
	})
}

func TestGlobalRateLimitFromEnvCanBeDisabled(t *testing.T) {
	t.Setenv("RATE_LIMIT_PER_MINUTE", "0")
	assert.Nil(t, GlobalRateLimitFromEnv(discardLogger()))

	t.Setenv("RATE_LIMIT_PER_MINUTE", "120")
	assert.NotNil(t, GlobalRateLimitFromEnv(discardLogger()))
}
