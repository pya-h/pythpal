# Pyth Price Feed Service

A lightweight Go server that fetches cryptocurrency prices from the [Pyth Network](https://pyth.network/) Hermes API, stores them in-memory, and serves them via REST and SSE endpoints.

## Features

- Batch-fetches prices from Pyth Hermes API on a configurable interval
- Auto-discovers all Pyth feeds when no token list is configured
- In-memory store with configurable history retention
- REST API for latest prices and short history
- SSE streaming for real-time price updates (with optional token filter)
- Stablecoin support (fixed price, no API call)
- Graceful shutdown

## Quick Start

```bash
# Build
make build

# Run with auto-discovered feeds
./pricefeed

# Run with specific tokens
TOKENS=BTC:0xe62df6c8b4a85fe1a67db44dc12de5db330f7ac66b72dc658afedf0f4a415b43,ETH:0xff61491a931112ddf1bd8147cd1b641375f79f5825126d665480874634fd0ace ./pricefeed
```

## Configuration

All configuration is via environment variables. See [.env.example](.env.example) for reference.

| Variable | Default | Description |
|---|---|---|
| `TOKENS` | *(auto-discover)* | Token list as `ID:pyth_feed_id` pairs, comma-separated. If omitted, all Pyth feeds are fetched. |
| `STABLE_COINS` | *(none)* | Comma-separated token IDs to treat as stablecoins (fixed price 1.00). |
| `FETCH_INTERVAL_SECONDS` | `60` | Seconds between price fetch cycles. |
| `SERVER_PORT` | `8080` | HTTP server port. |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error`. |
| `HISTORY_CAP_MINUTES` | `5` | Minutes of price history to retain per token. |
| `HERMES_BASE_URL` | `https://hermes.pyth.network` | Pyth Hermes API base URL. |

## API Endpoints

### `GET /health`
Health check. Returns `{"status": "ok"}`.

### `GET /prices`
Returns the latest price for every tracked token.

### `GET /prices/{token}`
Returns the latest price for a specific token. Returns 404 if not found.

### `GET /prices/{token}/history?limit=N`
Returns the last N price entries for a token (default limit: 60).

### `GET /events`
SSE stream of real-time price updates. Each event is a JSON-encoded price object.

Filter by token: `GET /events?tokens=BTC,ETH`

### Price Object

```json
{
  "token_id": "BTC",
  "price": 94500.12,
  "ema_price": 94600.50,
  "confidence": 0.001,
  "timestamp": "2025-01-15T12:00:00Z"
}
```

## Development

```bash
make test       # Run tests
make test-race  # Run tests with race detector
make build      # Build binary
make run        # Build and run
```
