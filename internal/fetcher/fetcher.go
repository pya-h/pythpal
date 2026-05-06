package fetcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/p4ya/umbra/pricefeed/internal/api"
	"github.com/p4ya/umbra/pricefeed/internal/config"
	"github.com/p4ya/umbra/pricefeed/internal/pyth"
	"github.com/p4ya/umbra/pricefeed/internal/store"
)

type Fetcher struct {
	client      pyth.PriceFetcher
	store       *store.Store
	broker      *api.Broker
	tokens      []config.TokenFeed
	stableCoins map[string]bool
	interval    time.Duration
}

func New(
	client pyth.PriceFetcher,
	s *store.Store,
	b *api.Broker,
	cfg *config.Config,
) *Fetcher {
	return &Fetcher{
		client:      client,
		store:       s,
		broker:      b,
		tokens:      cfg.Tokens,
		stableCoins: cfg.StableCoins,
		interval:    cfg.FetchInterval,
	}
}

// FetchOnce runs a single fetch cycle. Used for initial data population.
func (f *Fetcher) FetchOnce(ctx context.Context) {
	f.fetchAll(ctx)
}

// Run starts the fetch loop. It blocks until ctx is cancelled.
// Retries the initial fetch up to 3 times to ensure data is available on startup,
// then continues on a regular ticker interval.
func (f *Fetcher) Run(ctx context.Context) {
	const maxRetries = 3
	const retryDelay = 3 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return
		}
		if attempt > 1 {
			slog.Info("retrying initial price fetch", "attempt", attempt)
			time.Sleep(retryDelay)
		}
		n := f.fetchAll(ctx)
		if n > 0 {
			slog.Info("initial price fetch complete", "prices", n)
			break
		}
		slog.Warn("initial fetch returned no prices", "attempt", attempt)
	}

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("fetcher stopped")
			return
		case <-ticker.C:
			f.fetchAll(ctx)
		}
	}
}

func (f *Fetcher) fetchAll(ctx context.Context) int {
	stored := 0

	// Handle stablecoins (no API call needed)
	now := time.Now()
	for _, token := range f.tokens {
		if !f.stableCoins[token.ID] {
			continue
		}
		p := store.Price{
			TokenID:   token.ID,
			Price:     1.0,
			EMAPrice:  1.0,
			Conf:      0.0,
			Timestamp: now,
		}
		f.store.SetLatest(token.ID, p)
		f.broker.Publish(p)
		stored++
	}

	// Collect non-stablecoin feed IDs for batch fetch
	var feedIDs []string
	feedToToken := make(map[string]string) // feedID -> tokenID
	for _, token := range f.tokens {
		if f.stableCoins[token.ID] {
			continue
		}
		feedIDs = append(feedIDs, token.FeedID)
		feedToToken[token.FeedID] = token.ID
	}

	if len(feedIDs) == 0 {
		return stored
	}

	if ctx.Err() != nil {
		return stored
	}

	results, err := f.client.FetchLatestPrices(ctx, feedIDs)
	if err != nil {
		slog.Error("batch fetch failed", "error", err)
		return stored
	}

	for _, feed := range results {
		tokenID, ok := feedToToken[feed.ID]
		if !ok {
			continue
		}

		p, err := feed.ToStorePrice(tokenID)
		if err != nil {
			slog.Error("price conversion failed", "token", tokenID, "error", err)
			continue
		}

		f.store.SetLatest(tokenID, p)
		f.broker.Publish(p)
		stored++

		slog.Debug("price updated",
			"token", tokenID,
			"price", p.Price,
			"ema", p.EMAPrice,
		)
	}

	return stored
}
