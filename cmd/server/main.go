package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pya-h/pythpal/internal/api"
	"github.com/pya-h/pythpal/internal/config"
	"github.com/pya-h/pythpal/internal/fetcher"
	"github.com/pya-h/pythpal/internal/pyth"
	"github.com/pya-h/pythpal/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.LogLevel)

	tokenIDs := make([]string, len(cfg.Tokens))
	for i, t := range cfg.Tokens {
		tokenIDs[i] = t.ID
	}
	stables := make([]string, 0, len(cfg.StableCoins))
	for id := range cfg.StableCoins {
		stables = append(stables, id)
	}
	slog.Warn("config loaded",
		"port", cfg.ServerPort,
		"app_env", cfg.AppEnv,
		"log_level", cfg.LogLevel,
		"fetch_interval", cfg.FetchInterval,
		"history_cap_minutes", cfg.HistoryCap,
		"hermes_url", cfg.HermesBaseURL,
		"tokens", tokenIDs,
		"stable_coins", stables,
	)

	priceStore := store.New(cfg.HistoryCap)
	broker := api.NewBroker()
	pythClient := pyth.NewClient(cfg.HermesBaseURL)

	// Auto-discover feeds from Pyth if TOKENS not configured
	if len(cfg.Tokens) == 0 {
		slog.Info("no TOKENS configured, discovering feeds from Pyth...")
		feeds, err := pythClient.ListPriceFeeds(context.Background())
		if err != nil {
			slog.Error("failed to discover feeds from Pyth", "error", err)
			os.Exit(1)
		}
		for _, f := range feeds {
			cfg.Tokens = append(cfg.Tokens, config.TokenFeed{
				ID:     f.NormalizeName(),
				FeedID: f.ID,
			})
		}
		slog.Info("discovered feeds", "count", len(cfg.Tokens))
	}

	slog.Info("starting priceservice",
		"port", cfg.ServerPort,
		"tokens", len(cfg.Tokens),
		"interval", cfg.FetchInterval,
	)

	// HTTP routes
	mux := http.NewServeMux()
	handler := api.NewHandler(priceStore, 2*cfg.FetchInterval)
	handler.RegisterRoutes(mux)
	api.RegisterSSE(mux, broker)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0, // SSE needs no write timeout
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server first so health checks work immediately
	go func() {
		slog.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	// Start fetcher — initial fetch then ticker loop
	fetchCtx, fetchCancel := context.WithCancel(context.Background())
	defer fetchCancel()

	f := fetcher.New(pythClient, priceStore, broker, cfg)
	go f.Run(fetchCtx)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutdown signal received", "signal", sig)

	fetchCancel()
	broker.Close()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown error", "error", err)
	}

	slog.Info("server stopped")
}

func setupLogger(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))
}
