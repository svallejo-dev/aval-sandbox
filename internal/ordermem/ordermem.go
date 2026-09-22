// Package ordermem keeps orders in memory. Its Store is safe for concurrent
// use and never shares an order with its callers.
package ordermem

import (
	"context"
	"fmt"
	"sync"

	"github.com/svallejo-dev/aval-sandbox/internal/order"
)

// Store is an in-memory order repository.
type Store struct {
	mu     sync.RWMutex
	orders map[string]*order.Order
}

// New returns an empty Store.
func New() *Store { return &Store{orders: make(map[string]*order.Order)} }

// Create stores a new order. It fails with order.ErrExists if the ID is taken.
func (s *Store) Create(_ context.Context, o *order.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orders[o.ID()]; ok {
		return fmt.Errorf("create order %s: %w", o.ID(), order.ErrExists)
	}
	s.orders[o.ID()] = o.Clone()
	return nil
}

// Get returns a copy of the order with the given ID, or order.ErrNotFound.
func (s *Store) Get(_ context.Context, id string) (*order.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, ok := s.orders[id]
	if !ok {
		return nil, fmt.Errorf("get order %s: %w", id, order.ErrNotFound)
	}
	return o.Clone(), nil
}

// Update applies fn to a copy of the order and stores the copy only if fn
// succeeds, so a failed update leaves the order as it was. It returns the
// updated order. fn runs under the store's lock and must not call the store.
func (s *Store) Update(_ context.Context, id string, fn func(*order.Order) error) (*order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return nil, fmt.Errorf("update order %s: %w", id, order.ErrNotFound)
	}
	c := o.Clone()
	if err := fn(c); err != nil {
		return nil, fmt.Errorf("update order %s: %w", id, err)
	}
	s.orders[id] = c
	return c.Clone(), nil
}
