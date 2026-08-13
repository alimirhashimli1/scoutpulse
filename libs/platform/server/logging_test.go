package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logLines runs one request through the Logging middleware and returns
// whatever it wrote.
func logLines(t *testing.T, path string, status int) []string {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))

	out := strings.TrimSpace(buf.String())
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// TestLogging_SkipsSuccessfulProbes is about signal, not performance.
//
// A Docker healthcheck polls /health every few seconds and Prometheus scrapes
// /metrics; logging each one buries the requests a person actually wants to
// read under thousands of identical lines.
func TestLogging_SkipsSuccessfulProbes(t *testing.T) {
	for _, path := range []string{"/health", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			assert.Empty(t, logLines(t, path, http.StatusOK),
				"a successful probe carries no information and must not be logged")
		})
	}
}

// TestLogging_LogsFailingProbes is the other half: a health check that starts
// failing is exactly when you need the line.
func TestLogging_LogsFailingProbes(t *testing.T) {
	lines := logLines(t, "/health", http.StatusInternalServerError)

	require.Len(t, lines, 1, "a failing probe must still be logged")

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	assert.Equal(t, "/health", entry["path"])
	assert.EqualValues(t, 500, entry["status"])
}

func TestLogging_LogsOrdinaryRequests(t *testing.T) {
	lines := logLines(t, "/api/v1/players", http.StatusOK)

	require.Len(t, lines, 1)

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	assert.Equal(t, "http request", entry["msg"])
	assert.Equal(t, "/api/v1/players", entry["path"])
	assert.Equal(t, "GET", entry["method"])
	assert.EqualValues(t, 200, entry["status"])
}

// TestLogging_ProbeSkipIsExactMatch guards against the skip widening to any
// path that merely contains "health".
func TestLogging_ProbeSkipIsExactMatch(t *testing.T) {
	for _, path := range []string{"/api/v1/health-records", "/healthz", "/metrics/extra"} {
		t.Run(path, func(t *testing.T) {
			assert.Len(t, logLines(t, path, http.StatusOK), 1,
				"only the exact probe paths are skipped")
		})
	}
}
