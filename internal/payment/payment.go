// Package payment defines the payment aggregate: the amount captured for an
// order and the refunds issued against it. It depends on the standard
// library only.
package payment

import (
	"errors"
	"fmt"
	"slices"
)

// MaxKeyLength is the longest idempotency key a refund accepts.
const MaxKeyLength = 128

// Errors of the payment aggregate and of its repositories.
var (
	ErrInvalidPayment  = errors.New("invalid payment")
	ErrInvalidAmount   = errors.New("invalid amount")
	ErrInvalidKey      = errors.New("invalid idempotency key")
	ErrKeyReused       = errors.New("idempotency key reused with another amount")
	ErrExceedsCaptured = errors.New("refund exceeds the captured amount")
	ErrNotFound        = errors.New("payment not found")
	ErrExists          = errors.New("payment already exists")
)

// Refund is money returned against a payment, identified by the idempotency
// key it was requested with. Money is in minor units, such as cents.
type Refund struct {
	Key    string
	Amount int64
}

// Payment is the payment aggregate. Build one with New: the zero value is
// not a payment.
type Payment struct {
	id       string
	orderID  string
	refunds  []Refund
	captured int64
}

// New returns a payment that captured a positive amount for an order.
func New(id, orderID string, amount int64) (*Payment, error) {
	if id == "" || orderID == "" {
		return nil, fmt.Errorf("%w: empty payment or order ID", ErrInvalidPayment)
	}
	if amount <= 0 {
		return nil, fmt.Errorf("%w: captured %d, want more than 0", ErrInvalidAmount, amount)
	}
	return &Payment{id: id, orderID: orderID, captured: amount}, nil
}

// ID returns the payment's identifier.
func (p *Payment) ID() string { return p.id }

// OrderID returns the identifier of the order the payment is for.
func (p *Payment) OrderID() string { return p.orderID }

// Captured returns the amount the payment captured.
func (p *Payment) Captured() int64 { return p.captured }

// Refunds returns a copy of the refunds issued, in the order they were.
func (p *Payment) Refunds() []Refund { return slices.Clone(p.refunds) }

// Refunded is the sum of the refunds issued.
func (p *Payment) Refunded() int64 {
	var total int64
	for _, r := range p.refunds {
		total += r.Amount
	}
	return total
}

// Refund returns amount to the customer under an idempotency key. A key
// refunds at most once: repeating it with the same amount returns the
// original refund with replayed set and changes nothing, and repeating it
// with another amount fails with ErrKeyReused. A refund that would take the
// refunded total above the captured amount fails with ErrExceedsCaptured.
func (p *Payment) Refund(key string, amount int64) (r Refund, replayed bool, err error) {
	if key == "" || len(key) > MaxKeyLength {
		return Refund{}, false, fmt.Errorf("%w: %d bytes outside 1..%d", ErrInvalidKey, len(key), MaxKeyLength)
	}
	if amount <= 0 {
		return Refund{}, false, fmt.Errorf("%w: %d, want more than 0", ErrInvalidAmount, amount)
	}
	if i := slices.IndexFunc(p.refunds, func(r Refund) bool { return r.Key == key }); i >= 0 {
		if prev := p.refunds[i]; prev.Amount != amount {
			return Refund{}, false, fmt.Errorf("%w: key already refunded %d, not %d", ErrKeyReused, prev.Amount, amount)
		}
		return p.refunds[i], true, nil
	}
	if left := p.captured - p.Refunded(); amount > left {
		return Refund{}, false, fmt.Errorf("%w: %d requested, %d left", ErrExceedsCaptured, amount, left)
	}
	r = Refund{Key: key, Amount: amount}
	p.refunds = append(p.refunds, r)
	return r, false, nil
}

// Clone returns a deep copy of p, so repositories never share a payment
// with their callers.
func (p *Payment) Clone() *Payment {
	c := *p
	c.refunds = slices.Clone(p.refunds)
	return &c
}
