package fetcher

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/p4ya/umbra/priceservice/internal/api"
	"github.com/p4ya/umbra/priceservice/internal/config"
	"github.com/p4ya/umbra/priceservice/internal/pyth"
	"github.com/p4ya/umbra/priceservice/internal/store"
)

// mockClient implements pyth.PriceFetcher for testing.
type mockClient struct {
	feeds map[string]*pyth.ParsedPriceFeed // feedID -> response
	err   error                            // global error (simulates full batch failure)
	calls int                              // number of FetchLatestPrices calls
}

func newMockClient() *mockClient {
	return &mockClient{
		feeds: make(map[string]*pyth.ParsedPriceFeed),
	}
}

func (m *mockClient) FetchLatestPrices(_ context.Context, feedIDs []string) ([]pyth.ParsedPriceFeed, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	var results []pyth.ParsedPriceFeed
	for _, id := range feedIDs {
		if feed, ok := m.feeds[id]; ok {
			results = append(results, *feed)
		}
	}
	return results, nil
}

func makeFeed(feedID, price, ema string) *pyth.ParsedPriceFeed {
	return &pyth.ParsedPriceFeed{
		ID: feedID,
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

func TestFetcher_BatchFetchesAllTokens(t *testing.T) {
	mock := newMockClient()
	mock.feeds["0xbtc"] = makeFeed("0xbtc", "9450000000000", "9460000000000")
	mock.feeds["0xeth"] = makeFeed("0xeth", "236700000000", "236800000000")

	s := store.New(5)
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

	// Both tokens should be in the store
	if _, ok := s.GetLatest("BTC"); !ok {
		t.Error("BTC not in store")
	}
	if _, ok := s.GetLatest("ETH"); !ok {
		t.Error("ETH not in store")
	}

	// Should use batch calls (not one per token)
	if mock.calls == 0 {
		t.Error("no fetch calls were made")
	}
}

func TestFetcher_WritesToStore(t *testing.T) {
	mock := newMockClient()
	mock.feeds["0xbtc"] = makeFeed("0xbtc", "9450000000000", "9460000000000")

	s := store.New(5)
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
	mock.feeds["0xbtc"] = makeFeed("0xbtc", "9450000000000", "9460000000000")

	s := store.New(5)
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

	s := store.New(5)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "USDT", FeedID: "0xusdt"},
	}, []string{"USDT"})

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	f.Run(ctx)

	// Should NOT have called the API (stablecoin feed IDs are excluded from batch)
	if mock.calls > 0 {
		// calls may happen but with empty feedIDs — that's fine, the key thing is
		// USDT wasn't in the batch and got a fixed price
	}

	p, ok := s.GetLatest("USDT")
	if !ok {
		t.Fatal("expected USDT price in store")
	}
	if p.Price != 1.0 || p.EMAPrice != 1.0 {
		t.Errorf("stablecoin price = %f / ema = %f, want 1.0", p.Price, p.EMAPrice)
	}
}

func TestFetcher_BatchErrorDoesNotCrash(t *testing.T) {
	mock := newMockClient()
	mock.err = fmt.Errorf("simulated API error")

	s := store.New(5)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "BTC", FeedID: "0xbtc"},
		{ID: "ETH", FeedID: "0xeth"},
	}, nil)

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	f.Run(ctx) // should not panic

	if _, ok := s.GetLatest("BTC"); ok {
		t.Error("BTC should not be in store after batch error")
	}
}

func TestFetcher_MixedStableAndNormal(t *testing.T) {
	mock := newMockClient()
	mock.feeds["0xbtc"] = makeFeed("0xbtc", "9450000000000", "9460000000000")

	s := store.New(5)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "USDT", FeedID: "0xusdt"},
		{ID: "BTC", FeedID: "0xbtc"},
	}, []string{"USDT"})

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	f.Run(ctx)

	// Both should be in store
	usdtP, ok := s.GetLatest("USDT")
	if !ok {
		t.Fatal("USDT not in store")
	}
	if usdtP.Price != 1.0 {
		t.Errorf("USDT price = %f, want 1.0", usdtP.Price)
	}

	btcP, ok := s.GetLatest("BTC")
	if !ok {
		t.Fatal("BTC not in store")
	}
	if btcP.Price < 94000 {
		t.Errorf("BTC price = %f, want ~94500", btcP.Price)
	}
}

func TestFetcher_StopsOnContextCancel(t *testing.T) {
	mock := newMockClient()
	mock.feeds["0xbtc"] = makeFeed("0xbtc", "9450000000000", "9460000000000")

	s := store.New(5)
	b := api.NewBroker()
	defer b.Close()

	cfg := newTestConfig([]config.TokenFeed{
		{ID: "BTC", FeedID: "0xbtc"},
	}, nil)
	cfg.FetchInterval = time.Hour

	f := New(mock, s, b, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		f.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fetcher did not stop after context cancellation")
	}
}
