package order_test

import (
	"errors"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/svallejo-dev/aval-sandbox/internal/order"
)

// graph is the test's own model of the status graph, kept apart from the
// production table so that a change to either one shows up.
var graph = map[order.Status][]order.Status{
	order.Pending: {order.Paid, order.Cancelled},
	order.Paid:    {order.Shipped, order.Cancelled},
}

var statuses = []order.Status{order.Pending, order.Paid, order.Shipped, order.Cancelled}

// mustNew builds an order or stops the test.
func mustNew(t *testing.T, lines ...order.Line) *order.Order {
	t.Helper()
	o, err := order.New("o-1", lines)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return o
}

func TestTotal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		lines []order.Line
		want  int64
	}{
		{name: "ORD-F01 one line", lines: []order.Line{{SKU: "A", Quantity: 3, UnitPrice: 250}}, want: 750},
		{
			name:  "ORD-F01 several lines",
			lines: []order.Line{{SKU: "A", Quantity: 2, UnitPrice: 1500}, {SKU: "B", Quantity: 1, UnitPrice: 250}},
			want:  3250,
		},
		{
			name:  "ORD-F01 free line adds nothing",
			lines: []order.Line{{SKU: "A", Quantity: 1, UnitPrice: 999}, {SKU: "GIFT", Quantity: 5, UnitPrice: 0}},
			want:  999,
		},
		{
			name:  "ORD-F01 largest order fits",
			lines: slices.Repeat([]order.Line{{SKU: "MAX", Quantity: order.MaxQuantity, UnitPrice: order.MaxUnitPrice}}, order.MaxLines),
			want:  int64(order.MaxLines) * order.MaxQuantity * order.MaxUnitPrice,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := mustNew(t, tt.lines...).Total(); got != tt.want {
				t.Errorf("Total() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewRejectsInvalidLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		line order.Line
	}{
		{name: "ORD-N01 negative unit price", line: order.Line{SKU: "A", Quantity: 1, UnitPrice: -100}},
		{name: "ORD-N01 zero quantity", line: order.Line{SKU: "A", Quantity: 0, UnitPrice: 100}},
		{name: "ORD-N01 negative quantity", line: order.Line{SKU: "A", Quantity: -2, UnitPrice: 100}},
		{name: "ORD-N01 quantity above the limit", line: order.Line{SKU: "A", Quantity: order.MaxQuantity + 1, UnitPrice: 1}},
		{name: "ORD-N01 unit price above the limit", line: order.Line{SKU: "A", Quantity: 1, UnitPrice: order.MaxUnitPrice + 1}},
		{name: "empty SKU", line: order.Line{Quantity: 1, UnitPrice: 100}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := order.New("o-1", []order.Line{tt.line}); !errors.Is(err, order.ErrInvalidLine) {
				t.Errorf("New with %+v: err = %v, want ErrInvalidLine", tt.line, err)
			}
		})
	}
}

func TestAddLine(t *testing.T) {
	t.Parallel()
	t.Run("ORD-F01 added line counts in the total", func(t *testing.T) {
		t.Parallel()
		o := mustNew(t, order.Line{SKU: "A", Quantity: 2, UnitPrice: 1500}, order.Line{SKU: "B", Quantity: 1, UnitPrice: 250})
		if err := o.AddLine(order.Line{SKU: "C", Quantity: 3, UnitPrice: 100}); err != nil {
			t.Fatalf("AddLine: %v", err)
		}
		if got := o.Total(); got != 3550 {
			t.Errorf("Total() = %d, want 3550", got)
		}
	})
	t.Run("ORD-N01 invalid line leaves the total", func(t *testing.T) {
		t.Parallel()
		o := mustNew(t, order.Line{SKU: "A", Quantity: 1, UnitPrice: 500})
		if err := o.AddLine(order.Line{SKU: "B", Quantity: 0, UnitPrice: 100}); !errors.Is(err, order.ErrInvalidLine) {
			t.Fatalf("AddLine: err = %v, want ErrInvalidLine", err)
		}
		if got := o.Total(); got != 500 {
			t.Errorf("Total() = %d, want 500", got)
		}
	})
	t.Run("ORD-N03 paid order rejects new lines", func(t *testing.T) {
		t.Parallel()
		o := mustNew(t, order.Line{SKU: "A", Quantity: 1, UnitPrice: 500})
		if err := o.TransitionTo(order.Paid); err != nil {
			t.Fatalf("TransitionTo(paid): %v", err)
		}
		if err := o.AddLine(order.Line{SKU: "B", Quantity: 1, UnitPrice: 100}); !errors.Is(err, order.ErrNotPending) {
			t.Fatalf("AddLine: err = %v, want ErrNotPending", err)
		}
		if got := len(o.Lines()); got != 1 {
			t.Errorf("len(Lines()) = %d, want 1", got)
		}
	})
}

func TestTransitionTo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		before []order.Status // path from pending to the starting status
		to     order.Status
		ok     bool
	}{
		{name: "ORD-F02 pending to paid", to: order.Paid, ok: true},
		{name: "ORD-F02 pending to cancelled", to: order.Cancelled, ok: true},
		{name: "ORD-F02 paid to shipped", before: []order.Status{order.Paid}, to: order.Shipped, ok: true},
		{name: "ORD-F02 paid to cancelled", before: []order.Status{order.Paid}, to: order.Cancelled, ok: true},
		{name: "ORD-F02 pending cannot skip to shipped", to: order.Shipped},
		{name: "ORD-F02 paid cannot go back to pending", before: []order.Status{order.Paid}, to: order.Pending},
		{name: "ORD-N02 shipped order stays shipped", before: []order.Status{order.Paid, order.Shipped}, to: order.Cancelled},
		{name: "ORD-N02 cancelled order stays cancelled", before: []order.Status{order.Cancelled}, to: order.Paid},
		{name: "ORD-N02 cancelled order never reopens", before: []order.Status{order.Cancelled}, to: order.Pending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := mustNew(t, order.Line{SKU: "A", Quantity: 1, UnitPrice: 100})
			for _, s := range tt.before {
				if err := o.TransitionTo(s); err != nil {
					t.Fatalf("TransitionTo(%s): %v", s, err)
				}
			}
			from := o.Status()
			err := o.TransitionTo(tt.to)
			switch {
			case tt.ok && err != nil:
				t.Fatalf("TransitionTo(%s) from %s: %v", tt.to, from, err)
			case tt.ok && o.Status() != tt.to:
				t.Errorf("Status() = %s, want %s", o.Status(), tt.to)
			case !tt.ok && !errors.Is(err, order.ErrInvalidTransition):
				t.Fatalf("TransitionTo(%s) from %s: err = %v, want ErrInvalidTransition", tt.to, from, err)
			case !tt.ok && o.Status() != from:
				t.Errorf("Status() = %s after a rejected change, want %s", o.Status(), from)
			}
		})
	}
}

// TestOrderStateMachine drives an order with random sequences of valid and
// invalid operations and checks it against a model after every step.
func TestOrderStateMachine(t *testing.T) {
	t.Parallel()
	t.Run("ORD-I01 invariants hold for any sequence", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(rt *rapid.T) {
			line := rapid.Custom(func(t *rapid.T) order.Line {
				return order.Line{
					SKU:       rapid.StringMatching(`[A-Z]{0,4}`).Draw(t, "sku"),
					Quantity:  rapid.IntRange(-2, order.MaxQuantity+1).Draw(t, "quantity"),
					UnitPrice: rapid.Int64Range(-5, order.MaxUnitPrice+1).Draw(t, "unitPrice"),
				}
			})
			valid := func(l order.Line) bool {
				return l.SKU != "" && l.Quantity >= 1 && l.Quantity <= order.MaxQuantity &&
					l.UnitPrice >= 0 && l.UnitPrice <= order.MaxUnitPrice
			}

			first := line.Filter(valid).Draw(rt, "first")
			o, err := order.New("o-1", []order.Line{first})
			if err != nil {
				rt.Fatalf("New: %v", err)
			}
			status, total, count := order.Pending, int64(first.Quantity)*first.UnitPrice, 1

			rt.Repeat(map[string]func(*rapid.T){
				"addLine": func(t *rapid.T) {
					l := line.Draw(t, "line")
					want := status == order.Pending && count < order.MaxLines && valid(l)
					if err := o.AddLine(l); (err == nil) != want {
						t.Fatalf("AddLine(%+v) in %s with %d lines: err = %v, want success %t", l, status, count, err, want)
					}
					if want {
						total += int64(l.Quantity) * l.UnitPrice
						count++
					}
				},
				"transition": func(t *rapid.T) {
					to := rapid.SampledFrom(statuses).Draw(t, "to")
					want := slices.Contains(graph[status], to)
					if err := o.TransitionTo(to); (err == nil) != want {
						t.Fatalf("TransitionTo(%s) from %s: err = %v, want success %t", to, status, err, want)
					}
					if want {
						status = to
					}
				},
				"": func(t *rapid.T) {
					var sum int64
					for _, l := range o.Lines() {
						sum += l.Subtotal()
					}
					switch {
					case o.Status() != status:
						t.Fatalf("Status() = %s, model says %s", o.Status(), status)
					case o.Total() != sum || o.Total() != total:
						t.Fatalf("Total() = %d, sum of lines %d, model %d", o.Total(), sum, total)
					case o.Total() < 0:
						t.Fatalf("Total() = %d, negative", o.Total())
					}
				},
			})
		})
	})
}
