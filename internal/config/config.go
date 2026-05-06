package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type TokenFeed struct {
	ID     string
	FeedID string
}

type Config struct {
	Tokens        []TokenFeed
	StableCoins   map[string]bool
	FetchInterval time.Duration
	ServerPort    int
	LogLevel      string
	AppEnv        string // "development" or "production"
	HistoryCap    int    // max minutes of history to keep per token
	HermesBaseURL string
}

// loadEnvFile reads a .env file and sets any variables not already in the environment.
// Silently skips if the file doesn't exist.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Don't override existing env vars (explicit env takes precedence)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

func Load() (*Config, error) {
	loadEnvFile(".env")
	port, err := getEnvInt("SERVER_PORT", 3000)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_PORT: %w", err)
	}

	interval, err := getEnvInt("FETCH_INTERVAL_SECONDS", 60)
	if err != nil {
		return nil, fmt.Errorf("invalid FETCH_INTERVAL_SECONDS: %w", err)
	}

	historyCap, err := getEnvInt("HISTORY_CAP_MINUTES", 5)
	if err != nil {
		return nil, fmt.Errorf("invalid HISTORY_CAP_MINUTES: %w", err)
	}

	tokens, err := parseTokens(os.Getenv("TOKENS"))
	if err != nil {
		return nil, fmt.Errorf("invalid TOKENS: %w", err)
	}

	stableCoins := make(map[string]bool)
	if sc := os.Getenv("STABLE_COINS"); sc != "" {
		for _, id := range strings.Split(sc, ",") {
			stableCoins[strings.TrimSpace(id)] = true
		}
	}

	hermesURL := os.Getenv("HERMES_BASE_URL")
	if hermesURL == "" {
		hermesURL = "https://hermes.pyth.network"
	}

	appEnv := getEnv("APP_ENV", "development")

	// Derive log level: explicit LOG_LEVEL takes priority, otherwise based on APP_ENV
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		switch appEnv {
		case "production":
			logLevel = "warn"
		default:
			logLevel = "debug"
		}
	}

	return &Config{
		Tokens:        tokens,
		StableCoins:   stableCoins,
		FetchInterval: time.Duration(interval) * time.Second,
		ServerPort:    port,
		LogLevel:      logLevel,
		AppEnv:        appEnv,
		HistoryCap:    historyCap,
		HermesBaseURL: hermesURL,
	}, nil
}

// parseTokens parses "BTC:0xabc123,ETH:0xdef456" format
func parseTokens(raw string) ([]TokenFeed, error) {
	if raw == "" {
		return nil, nil
	}
	var tokens []TokenFeed
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid token:feed pair %q", pair)
		}
		tokens = append(tokens, TokenFeed{
			ID:     parts[0],
			FeedID: parts[1],
		})
	}
	return tokens, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return strconv.Atoi(v)
}
