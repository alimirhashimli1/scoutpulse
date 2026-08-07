package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRequestID(t *testing.T) {
	t.Run("generates an id when none is supplied", func(t *testing.T) {
		var seen string
		h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RequestIDFrom(r.Context())
		}))

		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.NotEmpty(t, seen)
		assert.Equal(t, seen, rr.Header().Get(RequestIDHeader))
	})

	t.Run("preserves an inbound id", func(t *testing.T) {
		// A call chain spanning several services must share one id, or the
		// logs cannot be correlated.
		var seen string
		h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RequestIDFrom(r.Context())
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeader, "upstream-id")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		assert.Equal(t, "upstream-id", seen)
		assert.Equal(t, "upstream-id", rr.Header().Get(RequestIDHeader))
	})
}

func TestRecover(t *testing.T) {
	h := Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rr := httptest.NewRecorder()
	require.NotPanics(t, func() {
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	})
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestCORS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("allowed origin is echoed", func(t *testing.T) {
		h := CORS([]string{"http://localhost:4200"})(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://localhost:4200")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		assert.Equal(t, "http://localhost:4200", rr.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, rr.Header().Values("Vary"), "Origin")
	})

	t.Run("disallowed origin gets no allow header", func(t *testing.T) {
		h := CORS([]string{"http://localhost:4200"})(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://evil.example")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("preflight short-circuits", func(t *testing.T) {
		called := false
		h := CORS([]string{"*"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			called = true
		}))

		req := httptest.NewRequest(http.MethodOptions, "/api/v1/teams", nil)
		req.Header.Set("Origin", "http://localhost:4200")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNoContent, rr.Code)
		assert.False(t, called, "preflight must not reach the route handler")
		assert.Contains(t, rr.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	})
}

func TestChainOrder(t *testing.T) {
	var order []string

	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	}), mark("outer"), mark("inner"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, []string{"outer", "inner", "handler"}, order)
}

func TestAllowedOriginsFromEnv(t *testing.T) {
	t.Run("defaults to wildcard", func(t *testing.T) {
		t.Setenv(CORSOriginsEnvVar, "")
		assert.Equal(t, []string{"*"}, allowedOriginsFromEnv())
	})

	t.Run("splits and trims", func(t *testing.T) {
		t.Setenv(CORSOriginsEnvVar, "http://a.example, http://b.example ")
		assert.Equal(t, []string{"http://a.example", "http://b.example"}, allowedOriginsFromEnv())
	})
}
