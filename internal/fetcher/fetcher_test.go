package fetcher

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/p4ya/umbra/pricefeed/internal/api"
	"github.com/p4ya/umbra/pricefeed/internal/config"
	"github.com/p4ya/umbra/pricefeed/internal/pyth"
	"github.com/p4ya/umbra/pricefeed/internal/store"
)

// mockClient implements pyth.PriceFetcher for testing.
type mockClient struct {
	feeds map[string]*pyth.ParsedPriceFeed // feedID -> response
	err   map[string]error                 // feedID -> error
	calls []string                         // records feedIDs that were called
}

func newMockClient() *mockClient {
	return &mockClient{
		feeds: make(map[string]*pyth.ParsedPriceFeed),
		err:   make(map[string]error),
	}
}

func (m *mockClient) FetchLatestPrice(_ context.Context, feedID string) (*pyth.ParsedPriceFeed, error) {
	m.calls = append(m.calls, feedID)
	if err, ok := m.err[feedID]; ok {
		return nil, err
	}
	if feed, ok := m.feeds[feedID]; ok {
		return feed, nil
	}
	return nil, fmt.Errorf("no mock data for feed %s", feedID)
}

func makeFeed(price, ema string) *pyth.ParsedPriceFeed {
	return &pyth.ParsedPriceFeed{
		Price: pyth.PriceData{
			Price:       price,
			Conf:        "100000",
			Expo:        -8,
			PublishTime: time.Now().Unix(),
		},
		EMAPrice: pyth.PriceData{
			Price:       ema,
			Conf:        "50000",
			Expo:        -8,
			PublishTime: time.Now().Unix(),
		},
	}
}

func newTestConfig(tokens []config.TokenFeed, stables []string) *config.Config {
	sc := make(map[string]bool)
	for _, s := range stables {
		sc[s] = true
	}
	return &config.Config{
		Tokens:        tokens,
		StableCoins:   sc,
		FetchInterval: 50 * time.Millisecond,
	}
}

func TestFetcher_CallsClientForEachToken(t *testing.T) {
	mock := newMockClient()
	mock.feeds["0xbtc"] = makeFeed("9450000000000", "9460000000000")
	mock.feeds["0xeth"] = makeFeed("236700000000", "236800000000")

	s := store.New(1440)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "BTC", FeedID: "0xbtc"},
		{ID: "ETH", FeedID: "0xeth"},
	}, nil)

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	f.Run(ctx)

	// Both feeds should have been called at least once
	btcCalls, ethCalls := 0, 0
	for _, id := range mock.calls {
		switch id {
		case "0xbtc":
			btcCalls++
		case "0xeth":
			ethCalls++
		}
	}
	if btcCalls == 0 {
		t.Error("BTC feed was never called")
	}
	if ethCalls == 0 {
		t.Error("ETH feed was never called")
	}
}

func TestFetcher_WritesToStore(t *testing.T) {
	mock := newMockClient()
	mock.feeds["0xbtc"] = makeFeed("9450000000000", "9460000000000")

	s := store.New(1440)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "BTC", FeedID: "0xbtc"},
	}, nil)

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	f.Run(ctx)

	p, ok := s.GetLatest("BTC")
	if !ok {
		t.Fatal("expected BTC price in store")
	}
	if p.Price < 94000 || p.Price > 95000 {
		t.Errorf("price = %f, want ~94500", p.Price)
	}
}

func TestFetcher_PublishesToBroker(t *testing.T) {
	mock := newMockClient()
	mock.feeds["0xbtc"] = makeFeed("9450000000000", "9460000000000")

	s := store.New(1440)
	b := api.NewBroker()
	defer b.Close()

	ch, unsub := b.Subscribe(nil)
	defer unsub()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "BTC", FeedID: "0xbtc"},
	}, nil)

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go f.Run(ctx)

	select {
	case p := <-ch:
		if p.TokenID != "BTC" {
			t.Errorf("got token %q, want BTC", p.TokenID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for broker event")
	}
}

func TestFetcher_StablecoinFixedPrice(t *testing.T) {
	mock := newMockClient()
	// No mock feed for USDT — it should never be called

	s := store.New(1440)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "USDT", FeedID: "0xusdt"},
	}, []string{"USDT"})

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	f.Run(ctx)

	// Should NOT have called the API
	for _, id := range mock.calls {
		if id == "0xusdt" {
			t.Error("stablecoin USDT should not call the API")
		}
	}

	// Should have a fixed price of 1.0 in the store
	p, ok := s.GetLatest("USDT")
	if !ok {
		t.Fatal("expected USDT price in store")
	}
	if p.Price != 1.0 || p.EMAPrice != 1.0 {
		t.Errorf("stablecoin price = %f / ema = %f, want 1.0", p.Price, p.EMAPrice)
	}
}

func TestFetcher_PerTokenErrorContinues(t *testing.T) {
	mock := newMockClient()
	mock.err["0xbtc"] = fmt.Errorf("simulated API error")
	mock.feeds["0xeth"] = makeFeed("236700000000", "236800000000")

	s := store.New(1440)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "BTC", FeedID: "0xbtc"},
		{ID: "ETH", FeedID: "0xeth"},
	}, nil)

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	f.Run(ctx)

	// BTC should fail, ETH should still succeed
	_, btcOK := s.GetLatest("BTC")
	if btcOK {
		t.Error("BTC should not be in store (API errored)")
	}

	_, ethOK := s.GetLatest("ETH")
	if !ethOK {
		t.Error("ETH should be in store despite BTC failure")
	}
}

func TestFetcher_StopsOnContextCancel(t *testing.T) {
	mock := newMockClient()
	mock.feeds["0xbtc"] = makeFeed("9450000000000", "9460000000000")

	s := store.New(1440)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "BTC", FeedID: "0xbtc"},
	}, nil)
	cfg.FetchInterval = time.Hour // very long interval

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		f.Run(ctx)
		close(done)
	}()

	// Let the initial fetch happen
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// good — fetcher exited
	case <-time.After(2 * time.Second):
		t.Fatal("fetcher did not stop after context cancellation")
	}
}
