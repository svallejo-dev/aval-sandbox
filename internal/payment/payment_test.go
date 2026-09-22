package payment_test

import (
	"errors"
	"testing"

	"pgregory.net/rapid"

	"github.com/svallejo-dev/aval-sandbox/internal/payment"
)

// mustNew builds a payment that captured amount, or stops the test.
func mustNew(t *testing.T, amount int64) *payment.Payment {
	t.Helper()
	p, err := payment.New("p-1", "o-1", amount)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// mustRefund refunds amount under key, or stops the test.
func mustRefund(t *testing.T, p *payment.Payment, key string, amount int64) (replayed bool) {
	t.Helper()
	_, replayed, err := p.Refund(key, amount)
	if err != nil {
		t.Fatalf("Refund(%q, %d): %v", key, amount, err)
	}
	return replayed
}

func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		amount  int64
		wantErr error
	}{
		{name: "ORD-F11 positive amount is captured", amount: 1000},
		{name: "ORD-F11 zero amount is rejected", amount: 0, wantErr: payment.ErrInvalidAmount},
		{name: "ORD-F11 negative amount is rejected", amount: -1, wantErr: payment.ErrInvalidAmount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := payment.New("p-1", "o-1", tt.amount)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("New(%d): err = %v, want %v", tt.amount, err, tt.wantErr)
			}
			if err == nil && (p.Captured() != tt.amount || p.Refunded() != 0) {
				t.Errorf("captured %d refunded %d, want %d and 0", p.Captured(), p.Refunded(), tt.amount)
			}
		})
	}
}

func TestRefund(t *testing.T) {
	t.Parallel()
	t.Run("ORD-F10 same key and amount refunds once", func(t *testing.T) {
		t.Parallel()
		p := mustNew(t, 1000)
		if mustRefund(t, p, "k1", 300) {
			t.Error("first refund reported as a replay")
		}
		if !mustRefund(t, p, "k1", 300) {
			t.Error("repeated refund not reported as a replay")
		}
		if got := p.Refunded(); got != 300 {
			t.Errorf("Refunded() = %d, want 300", got)
		}
	})
	t.Run("ORD-F10 different keys refund separately", func(t *testing.T) {
		t.Parallel()
		p := mustNew(t, 1000)
		mustRefund(t, p, "k1", 300)
		mustRefund(t, p, "k2", 200)
		if got := p.Refunded(); got != 500 {
			t.Errorf("Refunded() = %d, want 500", got)
		}
	})
	t.Run("ORD-F12 refund without a key is rejected", func(t *testing.T) {
		t.Parallel()
		p := mustNew(t, 1000)
		if _, _, err := p.Refund("", 100); !errors.Is(err, payment.ErrInvalidKey) {
			t.Fatalf("Refund without key: err = %v, want ErrInvalidKey", err)
		}
		if got := p.Refunded(); got != 0 {
			t.Errorf("Refunded() = %d, want 0", got)
		}
	})
	t.Run("ORD-N10 refund above what is left is rejected", func(t *testing.T) {
		t.Parallel()
		p := mustNew(t, 1000)
		mustRefund(t, p, "k1", 600)
		if _, _, err := p.Refund("k2", 500); !errors.Is(err, payment.ErrExceedsCaptured) {
			t.Fatalf("Refund above captured: err = %v, want ErrExceedsCaptured", err)
		}
		if got := p.Refunded(); got != 600 {
			t.Errorf("Refunded() = %d, want 600", got)
		}
	})
	t.Run("ORD-N11 reused key with another amount is rejected", func(t *testing.T) {
		t.Parallel()
		p := mustNew(t, 1000)
		mustRefund(t, p, "k1", 300)
		if _, _, err := p.Refund("k1", 400); !errors.Is(err, payment.ErrKeyReused) {
			t.Fatalf("Refund with reused key: err = %v, want ErrKeyReused", err)
		}
		if got := p.Refunded(); got != 300 {
			t.Errorf("Refunded() = %d, want 300", got)
		}
	})
}

// TestPaymentStateMachine drives a payment with random refunds, reusing keys
// on purpose, and checks it against a model after every step.
func TestPaymentStateMachine(t *testing.T) {
	t.Parallel()
	t.Run("ORD-I10 refunded never exceeds captured", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(rt *rapid.T) {
			captured := rapid.Int64Range(1, 10_000).Draw(rt, "captured")
			p, err := payment.New("p-1", "o-1", captured)
			if err != nil {
				rt.Fatalf("New(%d): %v", captured, err)
			}
			issued := make(map[string]int64) // model: amount refunded per key
			var refunded int64

			rt.Repeat(map[string]func(*rapid.T){
				"refund": func(t *rapid.T) {
					key := rapid.SampledFrom([]string{"k1", "k2", "k3", "k4", "k5"}).Draw(t, "key")
					amount := rapid.Int64Range(-1, captured+1).Draw(t, "amount")
					prev, seen := issued[key]
					_, replayed, err := p.Refund(key, amount)
					switch {
					case amount <= 0:
						if !errors.Is(err, payment.ErrInvalidAmount) {
							t.Fatalf("Refund(%q, %d): err = %v, want ErrInvalidAmount", key, amount, err)
						}
					case seen && prev == amount:
						if err != nil || !replayed {
							t.Fatalf("Refund(%q, %d) repeated: replayed %t, err = %v", key, amount, replayed, err)
						}
					case seen:
						if !errors.Is(err, payment.ErrKeyReused) {
							t.Fatalf("Refund(%q, %d) after %d: err = %v, want ErrKeyReused", key, amount, prev, err)
						}
					case refunded+amount > captured:
						if !errors.Is(err, payment.ErrExceedsCaptured) {
							t.Fatalf("Refund(%q, %d) with %d of %d refunded: err = %v, want ErrExceedsCaptured", key, amount, refunded, captured, err)
						}
					default:
						if err != nil || replayed {
							t.Fatalf("Refund(%q, %d): replayed %t, err = %v", key, amount, replayed, err)
						}
						issued[key] = amount
						refunded += amount
					}
				},
				"": func(t *rapid.T) {
					var sum int64
					for _, r := range p.Refunds() {
						sum += r.Amount
					}
					switch {
					case p.Refunded() != sum || p.Refunded() != refunded:
						t.Fatalf("Refunded() = %d, sum of refunds %d, model %d", p.Refunded(), sum, refunded)
					case p.Refunded() > p.Captured():
						t.Fatalf("Refunded() = %d above Captured() = %d", p.Refunded(), p.Captured())
					}
				},
			})
		})
	})
}
