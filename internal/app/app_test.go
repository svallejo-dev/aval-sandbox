package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/svallejo-dev/aval-sandbox/internal/app"
)

func TestHandlerRoutes(t *testing.T) {
	t.Parallel()
	h := app.Handler()
	for _, tt := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/healthz", http.StatusNoContent},
		{http.MethodGet, "/orders/nope", http.StatusNotFound},
		{http.MethodGet, "/payments/nope", http.StatusNotFound},
		{http.MethodDelete, "/orders/nope", http.StatusMethodNotAllowed},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
		if rec.Code != tt.want {
			t.Errorf("%s %s: status %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}
