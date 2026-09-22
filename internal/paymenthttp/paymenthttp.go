// Package paymenthttp exposes the payment aggregate over HTTP.
package paymenthttp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"

	"github.com/svallejo-dev/aval-sandbox/internal/payment"
	"github.com/svallejo-dev/aval-sandbox/internal/platform/httpserver"
)

// KeyHeader carries the idempotency key of a refund.
const KeyHeader = "Idempotency-Key"

var errMissingKey = errors.New("missing " + KeyHeader + " header")

// Repository stores payments.
type Repository interface {
	Create(ctx context.Context, p *payment.Payment) error
	Get(ctx context.Context, id string) (*payment.Payment, error)
	Update(ctx context.Context, id string, fn func(*payment.Payment) error) (*payment.Payment, error)
}

// Handler serves the /payments endpoints.
type Handler struct {
	repo Repository
}

// New returns a Handler that keeps its payments in repo.
func New(repo Repository) *Handler { return &Handler{repo: repo} }

// Register adds the payment routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /payments", h.capture)
	mux.HandleFunc("GET /payments/{id}", h.get)
	mux.HandleFunc("POST /payments/{id}/refunds", h.refund)
}

type refundResponse struct {
	Key    string `json:"key"`
	Amount int64  `json:"amount"`
}

type paymentResponse struct {
	ID       string           `json:"id"`
	OrderID  string           `json:"orderId"`
	Captured int64            `json:"captured"`
	Refunded int64            `json:"refunded"`
	Refunds  []refundResponse `json:"refunds"`
}

func newPaymentResponse(p *payment.Payment) paymentResponse {
	refunds := p.Refunds()
	resp := paymentResponse{
		ID: p.ID(), OrderID: p.OrderID(), Captured: p.Captured(), Refunded: p.Refunded(),
		Refunds: make([]refundResponse, 0, len(refunds)),
	}
	for _, r := range refunds {
		resp.Refunds = append(resp.Refunds, refundResponse(r))
	}
	return resp
}

func (h *Handler) capture(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string `json:"orderId"`
		Amount  int64  `json:"amount"`
	}
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		httpserver.Error(w, r, http.StatusBadRequest, err)
		return
	}
	p, err := payment.New(rand.Text(), req.OrderID, req.Amount)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := h.repo.Create(r.Context(), p); err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/payments/"+p.ID())
	httpserver.JSON(w, http.StatusCreated, newPaymentResponse(p))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.repo.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpserver.JSON(w, http.StatusOK, newPaymentResponse(p))
}

// refund issues a refund, 201 Created, or replays the one its key already
// issued, 200 OK.
func (h *Handler) refund(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get(KeyHeader)
	if key == "" {
		httpserver.Error(w, r, http.StatusBadRequest, errMissingKey)
		return
	}
	var req struct {
		Amount int64 `json:"amount"`
	}
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		httpserver.Error(w, r, http.StatusBadRequest, err)
		return
	}
	var (
		issued   payment.Refund
		replayed bool
	)
	_, err := h.repo.Update(r.Context(), r.PathValue("id"), func(p *payment.Payment) error {
		var err error
		if issued, replayed, err = p.Refund(key, req.Amount); err != nil {
			return fmt.Errorf("refund: %w", err)
		}
		return nil
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	httpserver.JSON(w, status, refundResponse(issued))
}

// fail writes err with the status its domain error calls for.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, payment.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, payment.ErrInvalidKey):
		status = http.StatusBadRequest
	case errors.Is(err, payment.ErrExceedsCaptured), errors.Is(err, payment.ErrExists):
		status = http.StatusConflict
	case errors.Is(err, payment.ErrInvalidPayment), errors.Is(err, payment.ErrInvalidAmount), errors.Is(err, payment.ErrKeyReused):
		status = http.StatusUnprocessableEntity
	}
	httpserver.Error(w, r, status, err)
}
