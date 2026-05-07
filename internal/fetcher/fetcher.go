package fetcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/pya-h/pythpal/internal/api"
	"github.com/pya-h/pythpal/internal/config"
	"github.com/pya-h/pythpal/internal/pyth"
	"github.com/pya-h/pythpal/internal/store"
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
			f.fetchWithRetry(ctx)
		}
	}
}

// fetchWithRetry attempts a fetch cycle and retries up to 2 times if no
// non-stablecoin prices were fetched (indicates API failure).
func (f *Fetcher) fetchWithRetry(ctx context.Context) {
	const maxRetries = 2
	const retryDelay = 5 * time.Second

	stableCount := 0
	marketCount := 0
	for _, token := range f.tokens {
		if f.stableCoins[token.ID] {
			stableCount++
		} else {
			marketCount++
		}
	}

	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		if ctx.Err() != nil {
			return
		}
		if attempt > 1 {
			slog.Warn("fetch cycle got no market prices, retrying", "attempt", attempt)
			time.Sleep(retryDelay)
		}
		n := f.fetchAll(ctx)
		if n > stableCount || marketCount == 0 {
			return
		}
	}
	slog.Error("fetch cycle failed after retries, will try again next interval")
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
