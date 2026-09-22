// Command orders serves the orders sandbox API, orders and their payments,
// kept in memory. It listens on ORDERS_ADDR, :8080 by default, and stops
// gracefully on SIGINT or SIGTERM.
package main

import (
	"cmp"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/svallejo-dev/aval-sandbox/internal/app"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := app.Run(ctx, cmp.Or(os.Getenv("ORDERS_ADDR"), ":8080"))
	stop()
	if err != nil {
		slog.Error("orders stopped", "err", err)
		os.Exit(1)
	}
}
