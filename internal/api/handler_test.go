package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/p4ya/umbra/pricefeed/internal/store"
)

func newTestHandler() (*Handler, *store.Store) {
	s := store.New(1440)
	h := NewHandler(s)
	return h, s
}

func newTestMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestHealth(t *testing.T) {
	h, _ := newTestHandler()
	mux := newTestMux(h)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestGetAllPrices_Empty(t *testing.T) {
	h, _ := newTestHandler()
	mux := newTestMux(h)

	req := httptest.NewRequest("GET", "/prices", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var prices []store.Price
	json.NewDecoder(rec.Body).Decode(&prices)
	if len(prices) != 0 {
		t.Errorf("got %d prices, want 0", len(prices))
	}
}

func TestGetAllPrices_Populated(t *testing.T) {
	h, s := newTestHandler()
	mux := newTestMux(h)

	s.SetLatest("BTC", store.Price{TokenID: "BTC", Price: 94500, Timestamp: time.Now()})
	s.SetLatest("ETH", store.Price{TokenID: "ETH", Price: 2400, Timestamp: time.Now()})

	req := httptest.NewRequest("GET", "/prices", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var prices []store.Price
	json.NewDecoder(rec.Body).Decode(&prices)
	if len(prices) != 2 {
		t.Errorf("got %d prices, want 2", len(prices))
	}
}

func TestGetPrice_Found(t *testing.T) {
	h, s := newTestHandler()
	mux := newTestMux(h)

	s.SetLatest("BTC", store.Price{TokenID: "BTC", Price: 94500, Timestamp: time.Now()})

	req := httptest.NewRequest("GET", "/prices/BTC", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var p store.Price
	json.NewDecoder(rec.Body).Decode(&p)
	if p.TokenID != "BTC" || p.Price != 94500 {
		t.Errorf("got %+v, want BTC@94500", p)
	}
}

func TestGetPrice_NotFound(t *testing.T) {
	h, _ := newTestHandler()
	mux := newTestMux(h)

	req := httptest.NewRequest("GET", "/prices/DOGE", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestGetHistory_DefaultLimit(t *testing.T) {
	h, s := newTestHandler()
	mux := newTestMux(h)

	for i := 0; i < 100; i++ {
		s.SetLatest("ETH", store.Price{
			TokenID:   "ETH",
			Price:     float64(2000 + i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	req := httptest.NewRequest("GET", "/prices/ETH/history", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var history []store.Price
	json.NewDecoder(rec.Body).Decode(&history)
	if len(history) != 60 {
		t.Errorf("got %d entries, want 60 (default limit)", len(history))
	}
}

func TestGetHistory_CustomLimit(t *testing.T) {
	h, s := newTestHandler()
	mux := newTestMux(h)

	for i := 0; i < 10; i++ {
		s.SetLatest("SOL", store.Price{
			TokenID:   "SOL",
			Price:     float64(150 + i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	req := httptest.NewRequest("GET", "/prices/SOL/history?limit=5", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var history []store.Price
	json.NewDecoder(rec.Body).Decode(&history)
	if len(history) != 5 {
		t.Errorf("got %d entries, want 5", len(history))
	}
}

func TestGetHistory_InvalidLimit(t *testing.T) {
	h, _ := newTestHandler()
	mux := newTestMux(h)

	req := httptest.NewRequest("GET", "/prices/ETH/history?limit=abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetHistory_Empty(t *testing.T) {
	h, _ := newTestHandler()
	mux := newTestMux(h)

	req := httptest.NewRequest("GET", "/prices/NONE/history", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var history []store.Price
	json.NewDecoder(rec.Body).Decode(&history)
	if len(history) != 0 {
		t.Errorf("got %d entries, want 0", len(history))
	}
}

func TestContentTypeJSON(t *testing.T) {
	h, _ := newTestHandler()
	mux := newTestMux(h)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}
