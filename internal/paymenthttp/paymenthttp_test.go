package paymenthttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/svallejo-dev/aval-sandbox/internal/paymenthttp"
	"github.com/svallejo-dev/aval-sandbox/internal/paymentmem"
)

// paymentBody is the JSON of a payment as clients see it.
type paymentBody struct {
	ID       string `json:"id"`
	OrderID  string `json:"orderId"`
	Captured int64  `json:"captured"`
	Refunded int64  `json:"refunded"`
}

// newServer returns the payment routes over an empty store.
func newServer() http.Handler {
	mux := http.NewServeMux()
	paymenthttp.New(paymentmem.New()).Register(mux)
	return mux
}

// do sends a request to h, with an idempotency key unless key is empty, and
// returns the recorded response.
func do(t *testing.T, h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(paymenthttp.KeyHeader, key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// capture posts a payment of amount and returns it, or stops the test.
func capture(t *testing.T, h http.Handler, amount string) paymentBody {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/payments", "", `{"orderId":"o-1","amount":`+amount+`}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /payments: status %d, body %s", rec.Code, rec.Body)
	}
	return decode(t, rec)
}

// refund posts a refund and returns its response status.
func refund(t *testing.T, h http.Handler, id, key, amount string) int {
	t.Helper()
	return do(t, h, http.MethodPost, "/payments/"+id+"/refunds", key, `{"amount":`+amount+`}`).Code
}

// refunded reads the refunded total of a payment.
func refunded(t *testing.T, h http.Handler, id string) int64 {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/payments/"+id, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /payments/%s: status %d, body %s", id, rec.Code, rec.Body)
	}
	return decode(t, rec).Refunded
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) paymentBody {
	t.Helper()
	var p paymentBody
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode payment: %v", err)
	}
	return p
}

func TestCapture(t *testing.T) {
	t.Parallel()
	t.Run("ORD-F11 positive amount is captured", func(t *testing.T) {
		t.Parallel()
		p := capture(t, newServer(), "1000")
		if p.ID == "" || p.OrderID != "o-1" || p.Captured != 1000 || p.Refunded != 0 {
			t.Errorf("payment = %+v, want captured 1000 for o-1 and nothing refunded", p)
		}
	})
	t.Run("ORD-F11 zero amount is rejected", func(t *testing.T) {
		t.Parallel()
		if rec := do(t, newServer(), http.MethodPost, "/payments", "", `{"orderId":"o-1","amount":0}`); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status %d, want 422; body %s", rec.Code, rec.Body)
		}
	})
}

func TestRefund(t *testing.T) {
	t.Parallel()
	t.Run("ORD-F10 replay returns the original refund", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		p := capture(t, h, "1000")
		if got := refund(t, h, p.ID, "k1", "300"); got != http.StatusCreated {
			t.Fatalf("first refund: status %d, want 201", got)
		}
		if got := refund(t, h, p.ID, "k1", "300"); got != http.StatusOK {
			t.Fatalf("replayed refund: status %d, want 200", got)
		}
		if got := refunded(t, h, p.ID); got != 300 {
			t.Errorf("refunded = %d, want 300", got)
		}
	})
	t.Run("ORD-F10 different keys refund separately", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		p := capture(t, h, "1000")
		for _, r := range []struct{ key, amount string }{{"k1", "300"}, {"k2", "200"}} {
			if got := refund(t, h, p.ID, r.key, r.amount); got != http.StatusCreated {
				t.Fatalf("refund %s: status %d, want 201", r.key, got)
			}
		}
		if got := refunded(t, h, p.ID); got != 500 {
			t.Errorf("refunded = %d, want 500", got)
		}
	})
	t.Run("ORD-F12 missing key is a bad request", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		p := capture(t, h, "1000")
		if got := refund(t, h, p.ID, "", "300"); got != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", got)
		}
		if got := refunded(t, h, p.ID); got != 0 {
			t.Errorf("refunded = %d, want 0", got)
		}
	})
	t.Run("ORD-F12 key longer than 128 bytes is a bad request", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		p := capture(t, h, "1000")
		if got := refund(t, h, p.ID, strings.Repeat("k", 129), "300"); got != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", got)
		}
		if got := refunded(t, h, p.ID); got != 0 {
			t.Errorf("refunded = %d, want 0", got)
		}
	})
	t.Run("ORD-F12 key of 128 bytes is accepted", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		p := capture(t, h, "1000")
		if got := refund(t, h, p.ID, strings.Repeat("k", 128), "300"); got != http.StatusCreated {
			t.Fatalf("status %d, want 201", got)
		}
		if got := refunded(t, h, p.ID); got != 300 {
			t.Errorf("refunded = %d, want 300", got)
		}
	})
	t.Run("ORD-N10 refund above captured is a conflict", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		p := capture(t, h, "1000")
		if got := refund(t, h, p.ID, "k1", "600"); got != http.StatusCreated {
			t.Fatalf("first refund: status %d, want 201", got)
		}
		if got := refund(t, h, p.ID, "k2", "500"); got != http.StatusConflict {
			t.Fatalf("second refund: status %d, want 409", got)
		}
		if got := refunded(t, h, p.ID); got != 600 {
			t.Errorf("refunded = %d, want 600", got)
		}
	})
	t.Run("ORD-N11 reused key with another amount is unprocessable", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		p := capture(t, h, "1000")
		if got := refund(t, h, p.ID, "k1", "300"); got != http.StatusCreated {
			t.Fatalf("first refund: status %d, want 201", got)
		}
		if got := refund(t, h, p.ID, "k1", "400"); got != http.StatusUnprocessableEntity {
			t.Fatalf("reused key: status %d, want 422", got)
		}
		if got := refunded(t, h, p.ID); got != 300 {
			t.Errorf("refunded = %d, want 300", got)
		}
	})
	t.Run("unknown payment is not found", func(t *testing.T) {
		t.Parallel()
		if got := refund(t, newServer(), "nope", "k1", "100"); got != http.StatusNotFound {
			t.Errorf("status %d, want 404", got)
		}
	})
}
