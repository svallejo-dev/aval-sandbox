package httpserver_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/svallejo-dev/aval-sandbox/internal/platform/httpserver"
)

func TestServeStopsWhenContextIsDone(t *testing.T) {
	t.Parallel()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- httpserver.Serve(ctx, ln, http.NotFoundHandler()) }()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET: status %d, want 404", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the context was done")
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		body    string
		wantErr bool
	}{
		{body: `{"name":"x"}`},
		{body: `{"name":"x","extra":1}`, wantErr: true},
		{body: `{"name":"x"} {}`, wantErr: true},
		{body: `{"name":`, wantErr: true},
	} {
		var v struct {
			Name string `json:"name"`
		}
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(tt.body))
		if err := httpserver.DecodeJSON(httptest.NewRecorder(), req, &v); (err != nil) != tt.wantErr {
			t.Errorf("DecodeJSON(%s): err = %v, want error %t", tt.body, err, tt.wantErr)
		}
	}
}
