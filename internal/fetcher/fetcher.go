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

// Run starts the fetch loop. It blocks until ctx is cancelled.
func (f *Fetcher) Run(ctx context.Context) {
	// Fetch immediately on start, then on every tick.
	f.fetchAll(ctx)

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

func (f *Fetcher) fetchAll(ctx context.Context) {
	for _, token := range f.tokens {
		if ctx.Err() != nil {
			return
		}

		p, err := f.fetchOne(ctx, token)
		if err != nil {
			slog.Error("fetch failed",
				"token", token.ID,
				"error", err,
			)
			continue
		}

		f.store.SetLatest(token.ID, p)
		f.broker.Publish(p)

		slog.Debug("price updated",
			"token", token.ID,
			"price", p.Price,
			"ema", p.EMAPrice,
		)
	}
}

func (f *Fetcher) fetchOne(ctx context.Context, token config.TokenFeed) (store.Price, error) {
	now := time.Now()

	if f.stableCoins[token.ID] {
		return store.Price{
			TokenID:   token.ID,
			Price:     1.0,
			EMAPrice:  1.0,
			Conf:      0.0,
			Timestamp: now,
		}, nil
	}

	feed, err := f.client.FetchLatestPrice(ctx, token.FeedID)
	if err != nil {
		return store.Price{}, err
	}

	return feed.ToStorePrice(token.ID)
}
