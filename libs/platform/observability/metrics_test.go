package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	call := func(h http.Handler, auth string) int {
		req := httptest.NewRequest(http.MethodGet, MetricsPath, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	if got := call(guard("", false, ok), ""); got != http.StatusOK {
		t.Errorf("private network, no token: want 200, got %d", got)
	}
	if got := call(guard("", true, ok), "Bearer "); got != http.StatusNotFound {
		t.Errorf("public, no token configured: want 404, got %d", got)
	}
	tokened := guard("s3cret", true, ok)
	if got := call(tokened, ""); got != http.StatusNotFound {
		t.Errorf("token configured, none sent: want 404, got %d", got)
	}
	if got := call(tokened, "Bearer wrong"); got != http.StatusNotFound {
		t.Errorf("wrong token: want 404, got %d", got)
	}
	if got := call(tokened, "Bearer s3cret"); got != http.StatusOK {
		t.Errorf("right token: want 200, got %d", got)
	}
}
