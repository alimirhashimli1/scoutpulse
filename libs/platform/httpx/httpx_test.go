package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusFor(t *testing.T) {
	tests := []struct {
		kind apperr.Kind
		want int
	}{
		{apperr.KindInvalid, http.StatusBadRequest},
		{apperr.KindUnauthorized, http.StatusUnauthorized},
		{apperr.KindForbidden, http.StatusForbidden},
		{apperr.KindNotFound, http.StatusNotFound},
		{apperr.KindConflict, http.StatusConflict},
		{apperr.KindInternal, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, StatusFor(tt.kind), tt.kind.String())
	}
}

// TestWriteError_DoesNotLeakCause is the guarantee that replaced the
// http.Error(w, err.Error(), 500) pattern: the internal cause is logged, and
// only the client-safe message is serialized.
func TestWriteError_DoesNotLeakCause(t *testing.T) {
	cause := errors.New(`pq: duplicate key value violates unique constraint "teams_name_key"`)
	err := apperr.Wrap(apperr.KindInternal, "could not save team", cause)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teams", nil)

	WriteError(rr, req, err)

	body := rr.Body.String()
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotContains(t, body, "teams_name_key")
	assert.NotContains(t, body, "pq:")
	assert.Contains(t, body, "could not save team")
}

// TestWriteError_UnknownErrorStaysGeneric covers errors that never passed
// through apperr: they must not be echoed back verbatim.
func TestWriteError_UnknownErrorStaysGeneric(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/players", nil)

	WriteError(rr, req, errors.New("dial tcp 10.0.0.5:5432: connect: connection refused"))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotContains(t, rr.Body.String(), "10.0.0.5")

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "internal server error", resp.Error)
	assert.Equal(t, "internal", resp.Code)
}

func TestWriteError_WrappedErrorKeepsKind(t *testing.T) {
	// A KindNotFound wrapped by fmt.Errorf must still map to 404.
	wrapped := errors.Join(errors.New("loading squad"), apperr.NotFound("team not found"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/x", nil)

	WriteError(rr, req, wrapped)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	t.Run("valid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Arsenal"}`))
		rr := httptest.NewRecorder()

		var got payload
		require.NoError(t, DecodeJSON(rr, req, &got))
		assert.Equal(t, "Arsenal", got.Name)
	})

	t.Run("unknown fields are rejected", func(t *testing.T) {
		// This is what stops a client from smuggling a field the struct
		// deliberately does not expose, such as a privileged role.
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Arsenal","role":"admin"}`))
		rr := httptest.NewRecorder()

		var got payload
		err := DecodeJSON(rr, req, &got)
		require.Error(t, err)
		assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
	})

	t.Run("malformed body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{not json`))
		rr := httptest.NewRecorder()

		var got payload
		err := DecodeJSON(rr, req, &got)
		require.Error(t, err)
		assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
	})

	t.Run("oversized body is rejected", func(t *testing.T) {
		huge := `{"name":"` + strings.Repeat("a", MaxRequestBodyBytes+1) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(huge))
		rr := httptest.NewRecorder()

		var got payload
		err := DecodeJSON(rr, req, &got)
		require.Error(t, err)
		assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
	})
}

func TestQueryHelpers(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?limit=50&free_agent=true&position=GK&bad=x", nil)

	limit, err := QueryInt(req, "limit", 25)
	require.NoError(t, err)
	assert.Equal(t, 50, limit)

	missing, err := QueryInt(req, "offset", 7)
	require.NoError(t, err)
	assert.Equal(t, 7, missing, "absent parameter falls back to the default")

	_, err = QueryInt(req, "bad", 0)
	require.Error(t, err)
	assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))

	freeAgent, err := QueryBool(req, "free_agent")
	require.NoError(t, err)
	require.NotNil(t, freeAgent)
	assert.True(t, *freeAgent)

	absent, err := QueryBool(req, "loaned")
	require.NoError(t, err)
	assert.Nil(t, absent, "absent boolean is nil, not false")

	assert.Equal(t, "GK", *QueryString(req, "position"))
	assert.Nil(t, QueryString(req, "nationality"))
}
