// Package paymentmem keeps payments in memory. Its Store is safe for
// concurrent use and never shares a payment with its callers.
package paymentmem

import (
	"context"
	"fmt"
	"sync"

	"github.com/svallejo-dev/aval-sandbox/internal/payment"
)

// Store is an in-memory payment repository.
type Store struct {
	mu       sync.RWMutex
	payments map[string]*payment.Payment
}

// New returns an empty Store.
func New() *Store { return &Store{payments: make(map[string]*payment.Payment)} }

// Create stores a new payment. It fails with payment.ErrExists if the ID is
// taken.
func (s *Store) Create(_ context.Context, p *payment.Payment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.payments[p.ID()]; ok {
		return fmt.Errorf("create payment %s: %w", p.ID(), payment.ErrExists)
	}
	s.payments[p.ID()] = p.Clone()
	return nil
}

// Get returns a copy of the payment with the given ID, or
// payment.ErrNotFound.
func (s *Store) Get(_ context.Context, id string) (*payment.Payment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.payments[id]
	if !ok {
		return nil, fmt.Errorf("get payment %s: %w", id, payment.ErrNotFound)
	}
	return p.Clone(), nil
}

// Update applies fn to a copy of the payment and stores the copy only if fn
// succeeds, so a failed update leaves the payment as it was. It returns the
// updated payment. fn runs under the store's lock and must not call the
// store.
func (s *Store) Update(_ context.Context, id string, fn func(*payment.Payment) error) (*payment.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[id]
	if !ok {
		return nil, fmt.Errorf("update payment %s: %w", id, payment.ErrNotFound)
	}
	c := p.Clone()
	if err := fn(c); err != nil {
		return nil, fmt.Errorf("update payment %s: %w", id, err)
	}
	s.payments[id] = c
	return c.Clone(), nil
}
