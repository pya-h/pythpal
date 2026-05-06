package pricefeed_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/p4ya/umbra/pricefeed/internal/api"
	"github.com/p4ya/umbra/pricefeed/internal/config"
	"github.com/p4ya/umbra/pricefeed/internal/fetcher"
	"github.com/p4ya/umbra/pricefeed/internal/pyth"
	"github.com/p4ya/umbra/pricefeed/internal/store"
)

// mockHermes returns a test HTTP server that mimics the Pyth Hermes API.
func mockHermes() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/updates/price/latest":
			resp := map[string]any{
				"parsed": []map[string]any{
					{
						"id": "0xbtcfeed",
						"price": map[string]any{
							"price": "9450000000000", "conf": "100000",
							"expo": -8, "publish_time": time.Now().Unix(),
						},
						"ema_price": map[string]any{
							"price": "9460000000000", "conf": "50000",
							"expo": -8, "publish_time": time.Now().Unix(),
						},
					},
					{
						"id": "0xethfeed",
						"price": map[string]any{
							"price": "236700000000", "conf": "80000",
							"expo": -8, "publish_time": time.Now().Unix(),
						},
						"ema_price": map[string]any{
							"price": "236800000000", "conf": "40000",
							"expo": -8, "publish_time": time.Now().Unix(),
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// startTestServer creates and starts a full server stack with a mock Pyth backend.
// Returns the server URL and a cleanup function.
func startTestServer(t *testing.T) (string, func()) {
	t.Helper()

	hermes := mockHermes()

	cfg := &config.Config{
		Tokens: []config.TokenFeed{
			{ID: "BTC", FeedID: "0xbtcfeed"},
			{ID: "ETH", FeedID: "0xethfeed"},
			{ID: "USDT", FeedID: "0xusdt"},
		},
		StableCoins:   map[string]bool{"USDT": true},
		FetchInterval: 100 * time.Millisecond,
		HistoryCap:    5,
		HermesBaseURL: hermes.URL,
	}

	priceStore := store.New(cfg.HistoryCap)
	broker := api.NewBroker()
	pythClient := pyth.NewClient(cfg.HermesBaseURL)

	mux := http.NewServeMux()
	handler := api.NewHandler(priceStore)
	handler.RegisterRoutes(mux)
	api.RegisterSSE(mux, broker)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	fetchCtx, fetchCancel := context.WithCancel(context.Background())
	f := fetcher.New(pythClient, priceStore, broker, cfg)
	go f.Run(fetchCtx)

	// Wait for at least one fetch cycle
	time.Sleep(250 * time.Millisecond)

	addr := fmt.Sprintf("http://%s", listener.Addr().String())

	cleanup := func() {
		fetchCancel()
		broker.Close()
		srv.Shutdown(context.Background())
		hermes.Close()
	}

	return addr, cleanup
}

func TestIntegration_Health(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(addr + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestIntegration_GetAllPrices(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(addr + "/prices")
	if err != nil {
		t.Fatalf("GET /prices: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var prices []store.Price
	json.NewDecoder(resp.Body).Decode(&prices)

	if len(prices) < 3 {
		t.Fatalf("got %d prices, want at least 3 (BTC, ETH, USDT)", len(prices))
	}

	found := make(map[string]bool)
	for _, p := range prices {
		found[p.TokenID] = true
	}
	for _, want := range []string{"BTC", "ETH", "USDT"} {
		if !found[want] {
			t.Errorf("missing price for %s", want)
		}
	}
}

func TestIntegration_GetSinglePrice(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(addr + "/prices/BTC")
	if err != nil {
		t.Fatalf("GET /prices/BTC: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var p store.Price
	json.NewDecoder(resp.Body).Decode(&p)

	if p.TokenID != "BTC" {
		t.Errorf("token_id = %q, want BTC", p.TokenID)
	}
	if p.Price < 94000 || p.Price > 95000 {
		t.Errorf("price = %f, want ~94500", p.Price)
	}
}

func TestIntegration_GetSinglePrice_NotFound(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(addr + "/prices/DOGE")
	if err != nil {
		t.Fatalf("GET /prices/DOGE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestIntegration_GetHistory(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(addr + "/prices/BTC/history?limit=10")
	if err != nil {
		t.Fatalf("GET /prices/BTC/history: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var history []store.Price
	json.NewDecoder(resp.Body).Decode(&history)

	if len(history) == 0 {
		t.Error("expected non-empty history")
	}
	for _, p := range history {
		if p.TokenID != "BTC" {
			t.Errorf("history entry has token_id %q, want BTC", p.TokenID)
		}
	}
}

func TestIntegration_Stablecoin(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(addr + "/prices/USDT")
	if err != nil {
		t.Fatalf("GET /prices/USDT: %v", err)
	}
	defer resp.Body.Close()

	var p store.Price
	json.NewDecoder(resp.Body).Decode(&p)

	if p.Price != 1.0 {
		t.Errorf("USDT price = %f, want 1.0", p.Price)
	}
	if p.EMAPrice != 1.0 {
		t.Errorf("USDT ema = %f, want 1.0", p.EMAPrice)
	}
}

func TestIntegration_SSE(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, addr+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	scanner := bufio.NewScanner(resp.Body)
	got := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var p store.Price
			if err := json.Unmarshal([]byte(line[6:]), &p); err != nil {
				t.Errorf("failed to parse SSE data: %v", err)
				continue
			}
			if p.TokenID != "" && p.Price > 0 {
				got = true
				break
			}
		}
	}
	if !got {
		t.Error("did not receive any valid SSE price event")
	}
}

func TestIntegration_SSE_TokenFilter(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, addr+"/events?tokens=ETH", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events?tokens=ETH: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var p store.Price
			if err := json.Unmarshal([]byte(line[6:]), &p); err != nil {
				continue
			}
			if p.TokenID == "BTC" {
				t.Error("received BTC event on ETH-filtered stream")
			}
			if p.TokenID == "ETH" {
				return // success
			}
		}
	}
	t.Error("did not receive ETH event on filtered stream")
}
