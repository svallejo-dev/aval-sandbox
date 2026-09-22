// Package httpserver runs the service's HTTP server and holds the JSON
// conventions its handlers share.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// maxBodyBytes caps the size of a request body.
const maxBodyBytes = 1 << 20

// shutdownTimeout is how long Run waits for in-flight requests to finish.
const shutdownTimeout = 10 * time.Second

// Run listens on addr and serves h until ctx is done, as Serve does.
func Run(ctx context.Context, addr string, h http.Handler) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	return Serve(ctx, ln, h)
}

// Serve serves h on ln until ctx is done, then shuts the server down
// gracefully, waiting for in-flight requests up to a timeout. It closes ln
// and returns nil after a clean shutdown.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	addr := ln.Addr().String()
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errc := make(chan error, 1) // buffered: the goroutine never blocks on it
	go func() { errc <- srv.Serve(ln) }()
	slog.InfoContext(ctx, "listening", "addr", addr)

	select {
	case err := <-errc:
		return fmt.Errorf("serve %s: %w", addr, err)
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("shut down %s: %w", addr, err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve %s: %w", addr, err)
	}
	return nil
}

// DecodeJSON decodes the body of r, at most 1 MiB, into v. Unknown fields
// and trailing data are errors.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode request body: unexpected data after the JSON value")
	}
	return nil
}

// JSON writes v as the JSON body of a response with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response body", "err", err)
	}
}

// errorBody is the JSON body of every error response.
type errorBody struct {
	Error string `json:"error"`
}

// Error writes err as a JSON error response with the given status. Server
// errors are logged and their details kept from the client.
func Error(w http.ResponseWriter, r *http.Request, status int, err error) {
	msg := err.Error()
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		msg = http.StatusText(status)
	}
	JSON(w, status, errorBody{Error: msg})
}
