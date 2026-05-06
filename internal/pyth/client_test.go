package pyth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const validHermesJSON = `{
	"parsed": [{
		"id": "ff61491a931112ddf1bd8147cd1b641375f79f5825126d665480874634fd0ace",
		"price": {
			"price": "236722004851",
			"conf": "138860300",
			"expo": -8,
			"publish_time": 1700000000
		},
		"ema_price": {
			"price": "239686652000",
			"conf": "97739563",
			"expo": -8,
			"publish_time": 1700000000
		}
	}]
}`

func TestFetchLatestPrice_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/updates/price/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(validHermesJSON))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	feed, err := client.FetchLatestPrice(context.Background(), "0xfeed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if feed.Price.Price != "236722004851" {
		t.Errorf("price = %q, want %q", feed.Price.Price, "236722004851")
	}
}

func TestFetchPriceAtTime_Success(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/updates/price/1700000000" {
			t.Errorf("unexpected path: %s, want /v2/updates/price/1700000000", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(validHermesJSON))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	feed, err := client.FetchPriceAtTime(context.Background(), "0xfeed", ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if feed.EMAPrice.Price != "239686652000" {
		t.Errorf("ema_price = %q, want %q", feed.EMAPrice.Price, "239686652000")
	}
}

func TestFetchLatestPrice_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.FetchLatestPrice(context.Background(), "0xfeed")
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
}

func TestFetchLatestPrice_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.FetchLatestPrice(context.Background(), "0xfeed")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestFetchLatestPrice_EmptyParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"parsed": []}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.FetchLatestPrice(context.Background(), "0xfeed")
	if err == nil {
		t.Fatal("expected error for empty parsed array, got nil")
	}
}

func TestFetchLatestPrice_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.FetchLatestPrice(context.Background(), "0xfeed")
	if err == nil {
		t.Fatal("expected error for 404 status, got nil")
	}
}

func TestFetchLatestPrice_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(validHermesJSON))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	client.httpClient.Timeout = 100 * time.Millisecond

	_, err := client.FetchLatestPrice(context.Background(), "0xfeed")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
