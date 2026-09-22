// Package order defines the order aggregate: its lines, its total and the
// statuses it moves through. It depends on the standard library only.
package order

import (
	"errors"
	"fmt"
	"slices"
)

// Limits on an order. Together they keep every total well inside int64.
const (
	MaxLines     = 100
	MaxQuantity  = 1_000
	MaxUnitPrice = 100_000_000 // minor units
)

// Status is where an order stands in its lifecycle.
type Status string

// Statuses of an order. Shipped and Cancelled are closed: nothing leaves them.
const (
	Pending   Status = "pending"
	Paid      Status = "paid"
	Shipped   Status = "shipped"
	Cancelled Status = "cancelled"
)

// edges is the status graph: the statuses each status may move to.
var edges = map[Status][]Status{
	Pending: {Paid, Cancelled},
	Paid:    {Shipped, Cancelled},
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case Pending, Paid, Shipped, Cancelled:
		return true
	}
	return false
}

// CanTransition reports whether an order may move from one status to another.
func CanTransition(from, to Status) bool { return slices.Contains(edges[from], to) }

// Errors of the order aggregate and of its repositories.
var (
	ErrInvalidOrder      = errors.New("invalid order")
	ErrInvalidLine       = errors.New("invalid order line")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrNotPending        = errors.New("order is not pending")
	ErrNotFound          = errors.New("order not found")
	ErrExists            = errors.New("order already exists")
)

// Line is one product of an order. Money is in minor units, such as cents.
type Line struct {
	SKU       string
	UnitPrice int64
	Quantity  int
}

// Subtotal is the line's quantity times its unit price.
func (l Line) Subtotal() int64 { return int64(l.Quantity) * l.UnitPrice }

// validate rejects the lines that could make a total negative or overflow.
func (l Line) validate() error {
	switch {
	case l.SKU == "":
		return fmt.Errorf("%w: empty SKU", ErrInvalidLine)
	case l.Quantity < 1 || l.Quantity > MaxQuantity:
		return fmt.Errorf("%w: quantity %d outside 1..%d", ErrInvalidLine, l.Quantity, MaxQuantity)
	case l.UnitPrice < 0 || l.UnitPrice > MaxUnitPrice:
		return fmt.Errorf("%w: unit price %d outside 0..%d", ErrInvalidLine, l.UnitPrice, MaxUnitPrice)
	}
	return nil
}

// Order is the order aggregate. Build one with New: the zero value is not
// an order.
type Order struct {
	id     string
	status Status
	lines  []Line
}

// New returns a pending order with between 1 and MaxLines valid lines.
func New(id string, lines []Line) (*Order, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: empty ID", ErrInvalidOrder)
	}
	if len(lines) == 0 || len(lines) > MaxLines {
		return nil, fmt.Errorf("%w: %d lines outside 1..%d", ErrInvalidOrder, len(lines), MaxLines)
	}
	for i, l := range lines {
		if err := l.validate(); err != nil {
			return nil, fmt.Errorf("line %d: %w", i, err)
		}
	}
	return &Order{id: id, status: Pending, lines: slices.Clone(lines)}, nil
}

// ID returns the order's identifier.
func (o *Order) ID() string { return o.id }

// Status returns the order's current status.
func (o *Order) Status() Status { return o.status }

// Lines returns a copy of the order's lines.
func (o *Order) Lines() []Line { return slices.Clone(o.lines) }

// Total is the sum of the subtotals of the order's lines.
func (o *Order) Total() int64 {
	var total int64
	for _, l := range o.lines {
		total += l.Subtotal()
	}
	return total
}

// AddLine appends a valid line to a pending order.
func (o *Order) AddLine(l Line) error {
	if o.status != Pending {
		return fmt.Errorf("add a line to a %s order: %w", o.status, ErrNotPending)
	}
	if len(o.lines) >= MaxLines {
		return fmt.Errorf("%w: already %d lines", ErrInvalidOrder, MaxLines)
	}
	if err := l.validate(); err != nil {
		return err
	}
	o.lines = append(o.lines, l)
	return nil
}

// TransitionTo moves the order to status to along an edge of the status
// graph. Any other change fails and leaves the status as it was.
func (o *Order) TransitionTo(to Status) error {
	if !CanTransition(o.status, to) {
		return fmt.Errorf("%w: %s to %s", ErrInvalidTransition, o.status, to)
	}
	o.status = to
	return nil
}

// Clone returns a deep copy of o, so repositories never share an order with
// their callers.
func (o *Order) Clone() *Order {
	c := *o
	c.lines = slices.Clone(o.lines)
	return &c
}
