package orderhttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/svallejo-dev/aval-sandbox/internal/orderhttp"
	"github.com/svallejo-dev/aval-sandbox/internal/ordermem"
)

// orderBody is the JSON of an order as clients see it.
type orderBody struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Lines  []struct {
		SKU      string `json:"sku"`
		Subtotal int64  `json:"subtotal"`
	} `json:"lines"`
	Total int64 `json:"total"`
}

// newServer returns the order routes over an empty store.
func newServer() http.Handler {
	mux := http.NewServeMux()
	orderhttp.New(ordermem.New()).Register(mux)
	return mux
}

// do sends a request to h and returns the recorded response.
func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decode reads the JSON order in rec, or stops the test.
func decode(t *testing.T, rec *httptest.ResponseRecorder) orderBody {
	t.Helper()
	var o orderBody
	if err := json.NewDecoder(rec.Body).Decode(&o); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	return o
}

// create posts an order with one line of 2 units at 1500 and returns it.
func create(t *testing.T, h http.Handler) orderBody {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/orders", `{"lines":[{"sku":"BOOK","quantity":2,"unitPrice":1500}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /orders: status %d, body %s", rec.Code, rec.Body)
	}
	return decode(t, rec)
}

// changeStatus moves an order to status, or stops the test.
func changeStatus(t *testing.T, h http.Handler, id, status string) {
	t.Helper()
	if rec := do(t, h, http.MethodPost, "/orders/"+id+"/status", `{"status":"`+status+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("change to %s: status %d, body %s", status, rec.Code, rec.Body)
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()
	t.Run("ORD-F04 create returns the order and its total", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		rec := do(t, h, http.MethodPost, "/orders", `{"lines":[{"sku":"BOOK","quantity":2,"unitPrice":1500}]}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status %d, want 201; body %s", rec.Code, rec.Body)
		}
		o := decode(t, rec)
		if got, want := rec.Header().Get("Location"), "/orders/"+o.ID; o.ID == "" || got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
		if o.Status != "pending" || o.Total != 3000 || len(o.Lines) != 1 || o.Lines[0].Subtotal != 3000 {
			t.Errorf("order = %+v, want pending with one line and total 3000", o)
		}
	})
	t.Run("ORD-N01 negative unit price is rejected", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		rec := do(t, h, http.MethodPost, "/orders", `{"lines":[{"sku":"BOOK","quantity":1,"unitPrice":-100}]}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status %d, want 422; body %s", rec.Code, rec.Body)
		}
	})
	t.Run("malformed body is a bad request", func(t *testing.T) {
		t.Parallel()
		if rec := do(t, newServer(), http.MethodPost, "/orders", `{"lines":`); rec.Code != http.StatusBadRequest {
			t.Errorf("status %d, want 400", rec.Code)
		}
	})
}

func TestGet(t *testing.T) {
	t.Parallel()
	t.Run("ORD-F03 get returns the stored order", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		want := create(t, h)
		rec := do(t, h, http.MethodGet, "/orders/"+want.ID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, want 200; body %s", rec.Code, rec.Body)
		}
		if got := decode(t, rec); got.ID != want.ID || got.Status != want.Status || got.Total != want.Total || len(got.Lines) != len(want.Lines) {
			t.Errorf("GET = %+v, want %+v", got, want)
		}
	})
	t.Run("ORD-F03 unknown order is not found", func(t *testing.T) {
		t.Parallel()
		if rec := do(t, newServer(), http.MethodGet, "/orders/nope", ""); rec.Code != http.StatusNotFound {
			t.Errorf("status %d, want 404", rec.Code)
		}
	})
}

func TestAddLine(t *testing.T) {
	t.Parallel()
	t.Run("ORD-F01 added line counts in the total", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		o := create(t, h)
		rec := do(t, h, http.MethodPost, "/orders/"+o.ID+"/lines", `{"sku":"PEN","quantity":3,"unitPrice":100}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, want 200; body %s", rec.Code, rec.Body)
		}
		if got := decode(t, rec).Total; got != 3300 {
			t.Errorf("total = %d, want 3300", got)
		}
	})
	t.Run("ORD-N03 paid order rejects new lines", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		o := create(t, h)
		changeStatus(t, h, o.ID, "paid")
		rec := do(t, h, http.MethodPost, "/orders/"+o.ID+"/lines", `{"sku":"PEN","quantity":1,"unitPrice":100}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status %d, want 409; body %s", rec.Code, rec.Body)
		}
		if got := decode(t, do(t, h, http.MethodGet, "/orders/"+o.ID, "")); got.Total != o.Total || len(got.Lines) != 1 {
			t.Errorf("order changed to %+v", got)
		}
	})
}

func TestChangeStatus(t *testing.T) {
	t.Parallel()
	t.Run("ORD-F02 pending order is paid", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		o := create(t, h)
		rec := do(t, h, http.MethodPost, "/orders/"+o.ID+"/status", `{"status":"paid"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, want 200; body %s", rec.Code, rec.Body)
		}
		if got := decode(t, rec).Status; got != "paid" {
			t.Errorf("status = %q, want paid", got)
		}
	})
	t.Run("ORD-F02 pending order cannot skip to shipped", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		o := create(t, h)
		if rec := do(t, h, http.MethodPost, "/orders/"+o.ID+"/status", `{"status":"shipped"}`); rec.Code != http.StatusConflict {
			t.Fatalf("status %d, want 409; body %s", rec.Code, rec.Body)
		}
		if got := decode(t, do(t, h, http.MethodGet, "/orders/"+o.ID, "")).Status; got != "pending" {
			t.Errorf("status = %q, want pending", got)
		}
	})
	t.Run("ORD-N02 shipped order is never cancelled", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		o := create(t, h)
		changeStatus(t, h, o.ID, "paid")
		changeStatus(t, h, o.ID, "shipped")
		if rec := do(t, h, http.MethodPost, "/orders/"+o.ID+"/status", `{"status":"cancelled"}`); rec.Code != http.StatusConflict {
			t.Fatalf("status %d, want 409; body %s", rec.Code, rec.Body)
		}
		if got := decode(t, do(t, h, http.MethodGet, "/orders/"+o.ID, "")).Status; got != "shipped" {
			t.Errorf("status = %q, want shipped", got)
		}
	})
	t.Run("unknown status is unprocessable", func(t *testing.T) {
		t.Parallel()
		h := newServer()
		o := create(t, h)
		if rec := do(t, h, http.MethodPost, "/orders/"+o.ID+"/status", `{"status":"lost"}`); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status %d, want 422", rec.Code)
		}
	})
}
