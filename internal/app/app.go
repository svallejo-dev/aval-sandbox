// Package app is the composition root of the orders service: it wires the
// repositories, the HTTP handlers and the server.
package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/svallejo-dev/aval-sandbox/internal/orderhttp"
	"github.com/svallejo-dev/aval-sandbox/internal/ordermem"
	"github.com/svallejo-dev/aval-sandbox/internal/paymenthttp"
	"github.com/svallejo-dev/aval-sandbox/internal/paymentmem"
	"github.com/svallejo-dev/aval-sandbox/internal/platform/httpserver"
)

// Handler returns every route of the service over empty in-memory
// repositories.
func Handler() http.Handler {
	mux := http.NewServeMux()
	orderhttp.New(ordermem.New()).Register(mux)
	paymenthttp.New(paymentmem.New()).Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// Run serves the service on addr until ctx is done.
func Run(ctx context.Context, addr string) error {
	if err := httpserver.Run(ctx, addr, Handler()); err != nil {
		return fmt.Errorf("orders service: %w", err)
	}
	return nil
}
