package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pya-h/pythpal/internal/store"
)

func TestSSE_ContentType(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	mux := http.NewServeMux()
	RegisterSSE(mux, broker)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}
}

func TestSSE_ReceivesEvents(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	mux := http.NewServeMux()
	RegisterSSE(mux, broker)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Give the SSE handler a moment to subscribe
	time.Sleep(50 * time.Millisecond)

	// Publish a price
	broker.Publish(store.Price{
		TokenID:   "BTC",
		Price:     94500,
		Timestamp: time.Now(),
	})

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var p store.Price
			if err := json.Unmarshal([]byte(data), &p); err != nil {
				t.Fatalf("failed to parse SSE data: %v", err)
			}
			if p.TokenID != "BTC" || p.Price != 94500 {
				t.Errorf("got %+v, want BTC@94500", p)
			}
			return // success
		}
	}
	t.Fatal("no SSE data line received")
}

func TestSSE_TokenFilter(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	mux := http.NewServeMux()
	RegisterSSE(mux, broker)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/events?tokens=ETH", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	// Publish BTC (should be filtered) then ETH (should arrive)
	broker.Publish(store.Price{TokenID: "BTC", Price: 94500, Timestamp: time.Now()})
	broker.Publish(store.Price{TokenID: "ETH", Price: 2400, Timestamp: time.Now()})

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var p store.Price
			json.Unmarshal([]byte(data), &p)
			if p.TokenID == "BTC" {
				t.Error("received BTC but filter was set to ETH only")
			}
			if p.TokenID == "ETH" {
				return // success
			}
		}
	}
	t.Fatal("did not receive ETH event")
}

func TestSSE_ClientDisconnect(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	mux := http.NewServeMux()
	RegisterSSE(mux, broker)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Cancel context to simulate client disconnect
	cancel()

	// Give the handler time to clean up
	time.Sleep(100 * time.Millisecond)

	// Publishing after disconnect should not panic or block
	broker.Publish(store.Price{TokenID: "BTC", Price: 94500, Timestamp: time.Now()})
}
