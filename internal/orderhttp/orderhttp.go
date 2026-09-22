// Package orderhttp exposes the order aggregate over HTTP.
package orderhttp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"

	"github.com/svallejo-dev/aval-sandbox/internal/order"
	"github.com/svallejo-dev/aval-sandbox/internal/platform/httpserver"
)

// Repository stores orders.
type Repository interface {
	Create(ctx context.Context, o *order.Order) error
	Get(ctx context.Context, id string) (*order.Order, error)
	Update(ctx context.Context, id string, fn func(*order.Order) error) (*order.Order, error)
}

// Handler serves the /orders endpoints.
type Handler struct {
	repo Repository
}

// New returns a Handler that keeps its orders in repo.
func New(repo Repository) *Handler { return &Handler{repo: repo} }

// Register adds the order routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", h.create)
	mux.HandleFunc("GET /orders/{id}", h.get)
	mux.HandleFunc("POST /orders/{id}/lines", h.addLine)
	mux.HandleFunc("POST /orders/{id}/status", h.changeStatus)
}

type lineRequest struct {
	SKU       string `json:"sku"`
	Quantity  int    `json:"quantity"`
	UnitPrice int64  `json:"unitPrice"`
}

func (l lineRequest) line() order.Line {
	return order.Line{SKU: l.SKU, Quantity: l.Quantity, UnitPrice: l.UnitPrice}
}

type lineResponse struct {
	SKU       string `json:"sku"`
	Quantity  int    `json:"quantity"`
	UnitPrice int64  `json:"unitPrice"`
	Subtotal  int64  `json:"subtotal"`
}

type orderResponse struct {
	ID     string         `json:"id"`
	Status order.Status   `json:"status"`
	Lines  []lineResponse `json:"lines"`
	Total  int64          `json:"total"`
}

func newOrderResponse(o *order.Order) orderResponse {
	lines := o.Lines()
	resp := orderResponse{ID: o.ID(), Status: o.Status(), Lines: make([]lineResponse, 0, len(lines)), Total: o.Total()}
	for _, l := range lines {
		resp.Lines = append(resp.Lines, lineResponse{SKU: l.SKU, Quantity: l.Quantity, UnitPrice: l.UnitPrice, Subtotal: l.Subtotal()})
	}
	return resp
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lines []lineRequest `json:"lines"`
	}
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		httpserver.Error(w, r, http.StatusBadRequest, err)
		return
	}
	lines := make([]order.Line, 0, len(req.Lines))
	for _, l := range req.Lines {
		lines = append(lines, l.line())
	}
	o, err := order.New(rand.Text(), lines)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := h.repo.Create(r.Context(), o); err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/orders/"+o.ID())
	httpserver.JSON(w, http.StatusCreated, newOrderResponse(o))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	o, err := h.repo.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpserver.JSON(w, http.StatusOK, newOrderResponse(o))
}

func (h *Handler) addLine(w http.ResponseWriter, r *http.Request) {
	var req lineRequest
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		httpserver.Error(w, r, http.StatusBadRequest, err)
		return
	}
	o, err := h.repo.Update(r.Context(), r.PathValue("id"), func(o *order.Order) error {
		return o.AddLine(req.line())
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	httpserver.JSON(w, http.StatusOK, newOrderResponse(o))
}

func (h *Handler) changeStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status order.Status `json:"status"`
	}
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		httpserver.Error(w, r, http.StatusBadRequest, err)
		return
	}
	if !req.Status.Valid() {
		httpserver.Error(w, r, http.StatusUnprocessableEntity, fmt.Errorf("unknown status %q", req.Status))
		return
	}
	o, err := h.repo.Update(r.Context(), r.PathValue("id"), func(o *order.Order) error {
		return o.TransitionTo(req.Status)
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	httpserver.JSON(w, http.StatusOK, newOrderResponse(o))
}

// fail writes err with the status its domain error calls for.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, order.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, order.ErrInvalidOrder), errors.Is(err, order.ErrInvalidLine):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, order.ErrInvalidTransition), errors.Is(err, order.ErrNotPending), errors.Is(err, order.ErrExists):
		status = http.StatusConflict
	}
	httpserver.Error(w, r, status, err)
}
