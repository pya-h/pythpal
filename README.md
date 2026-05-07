# Price Service

A lightweight Go server that fetches cryptocurrency prices from the [Pyth Network](https://pyth.network/) Hermes API, stores them in-memory, and serves them via REST and SSE endpoints. Built for services and bots that need fast, low-latency access to current crypto prices without querying Pyth directly.

## How It Works

The server runs a background loop that batch-fetches prices from the Pyth Hermes oracle every N seconds. Prices are stored in-memory (no database) and served over HTTP. Bots and services can either poll the REST API or subscribe to real-time price updates via SSE.

```
Pyth Hermes API
      |
      v
  [Fetcher]  -- every 60s, batch-fetches all feeds -->  [In-Memory Store]
      |                                                       |
      v                                                       v
  [SSE Broker]  -- pushes to subscribers              [REST API]  -- serves on demand
      |                                                       |
      v                                                       v
  GET /events (real-time stream)                  GET /prices, /prices/{token}
```

## Features

- **Auto-discovery**: if no token list is configured, discovers all available Pyth feeds automatically
- **Batch fetching**: fetches all prices in a single HTTP call per tick (chunked at 100 feeds)
- **In-memory store**: keeps the last 5 minutes of prices per token (configurable)
- **REST API**: latest prices, single token lookup, short history
- **SSE streaming**: real-time push with optional token filter -- ideal for bots
- **Stablecoin handling**: stablecoins get a fixed price of 1.00, no API call
- **Graceful shutdown**: SIGINT/SIGTERM handling with clean resource cleanup
- **Zero dependencies**: built entirely on Go's standard library

## Quick Start

```bash
make build
./priceservice
```

Or with specific tokens:

```bash
export DESIRED_TOKENS=BTC:0xe62df6c8b4a85fe1a67db44dc12de5db330f7ac66b72dc658afedf0f4a415b43,ETH:0xff61491a931112ddf1bd8147cd1b641375f79f5825126d665480874634fd0ace
export STABLE_COINS=USDT,USDC
./priceservice
```

## Configuration

All via environment variables. See [.env.example](.env.example).

| Variable | Default | Description |
|---|---|---|
| `APP_ENV` | `development` | `development` (debug logs) or `production` (warn+ only). |
| `DESIRED_TOKENS` | *(auto-discover)* | `ID:pyth_feed_id` pairs, comma-separated. Omit to fetch all Pyth feeds. |
| `STABLE_COINS` | *(none)* | Token IDs to treat as stablecoins (fixed price 1.00). |
| `FETCH_INTERVAL_SECONDS` | `60` | Seconds between fetch cycles. |
| `SERVER_PORT` | `3000` | HTTP server port. |
| `LOG_LEVEL` | *(from APP_ENV)* | Override: `debug`, `info`, `warn`, `error`. |
| `HISTORY_CAP_MINUTES` | `5` | Minutes of price history to keep per token. |
| `HERMES_BASE_URL` | `https://hermes.pyth.network` | Pyth Hermes API base URL. |

## API

### `GET /health`
Returns `{"status": "ok"}`.

### `GET /prices`
All latest prices as a JSON array.

**Query params** (mutually exclusive):
- `tokens=BTC,ETH,SOL` — filter by specific token IDs
- `skip=N&take=M` — pagination (cannot combine with `tokens`)

### `GET /prices/{token}`
Latest price for a single token. 404 if not found.

### `GET /prices/{token}/history?limit=N`
Last N price entries for a token (default: 60, most recent last).

### `GET /events`
SSE stream of price updates. Each event is a `data:` line with a JSON price object.

Filter: `GET /events?tokens=BTC,ETH`

### Price Object

```json
{
  "token_id": "BTC",
  "price": 94500.12,
  "ema_price": 94600.50,
  "confidence": 0.001,
  "timestamp": "2025-01-15T12:00:00Z",
  "stale": false
}
```

| Field | Description |
|---|---|
| `token_id` | Token identifier (e.g. `BTC`, `BTC-EUR` for non-USD pairs) |
| `price` | Current spot price (normalized from Pyth's raw integer + exponent) |
| `ema_price` | Exponential moving average price |
| `confidence` | Price confidence interval |
| `timestamp` | Pyth publish time |
| `stale` | `true` if the price is older than 2x the fetch interval (omitted when fresh) |

## Development

```bash
make test       # run unit + integration tests
make test-race  # run all tests with race detector
make build      # build binary
make run        # build and run
make clean      # remove binary
```

### Running Tests

All tests (unit + integration) run with the standard Go toolchain:

```bash
go test ./...           # all tests
go test -race ./...     # with race detector (recommended)
go test -v ./...        # verbose output
```

Integration tests (`integration_test.go`) spin up a full server stack with a mock Hermes backend — no external dependencies or network access required.