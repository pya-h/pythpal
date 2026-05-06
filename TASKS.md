# Pyth Price Feed Service — Development Plan

## Phase 1: Project Scaffolding & Data Models

- [ ] Initialize Go module (`go mod init`)
- [ ] Create directory structure:
  ```
  cmd/server/main.go
  internal/pyth/
  internal/store/
  internal/api/
  internal/config/
  ```
- [ ] Define configuration struct & loader (`internal/config/config.go`)
  - Token list (id -> pyth feed id mapping)
  - Fetch interval
  - Server port
  - Log level
  - Load from environment variables / `.env`
- [ ] Define core data model (`internal/store/model.go`)
  ```go
  type Price struct {
      TokenID   string
      Price     float64
      EMAPrice  float64
      Conf      float64
      Timestamp time.Time
  }
  ```
- [ ] Create `.env.example` with sample values

## Phase 2: Pyth Hermes Client

- [ ] Implement Hermes REST client (`internal/pyth/client.go`)
  - Struct with base URL and `*http.Client`
  - Method: `FetchLatestPrice(ctx, feedID) (PythResponse, error)`
  - Method: `FetchPriceAtTime(ctx, feedID, timestamp) (PythResponse, error)`
  - Parse Hermes JSON response, extract price/ema/confidence/expo fields
  - Normalize raw price using exponent (price * 10^expo)
- [ ] Define Pyth-specific response types (`internal/pyth/types.go`)
  - Raw API response structs (for JSON unmarshaling)
  - Conversion method from raw response to `store.Price`

## Phase 3: Tests — Pyth Client

- [ ] Unit tests for price normalization / exponent math
- [ ] Unit tests for JSON response parsing (use fixture JSON files or inline constants)
- [ ] HTTP tests using `httptest.NewServer` to mock Hermes API
  - Successful response
  - Malformed JSON
  - HTTP errors (500, 404, timeout)
  - Empty / missing price data

## Phase 4: In-Memory Price Store

- [ ] Implement store (`internal/store/store.go`)
  - `Store` struct: `sync.RWMutex` + map of token ID -> price slice
  - `SetLatest(tokenID, Price)` — appends to history, updates latest
  - `GetLatest(tokenID) (Price, bool)` — returns most recent price
  - `GetAllLatest() map[string]Price` — returns latest for all tokens
  - `GetHistory(tokenID, limit int) []Price` — returns last N entries
- [ ] Implement history cap — keep at most N minutes of history per token (configurable, e.g. 1440 = 24h)
  - Prune old entries on each write

## Phase 5: Tests — Store

- [ ] Test `SetLatest` and `GetLatest` basic flow
- [ ] Test `GetAllLatest` with multiple tokens
- [ ] Test `GetHistory` returns correct order and respects limit
- [ ] Test history cap / pruning — insert more than max, verify old entries dropped
- [ ] Test concurrent read/write safety (launch goroutines doing parallel reads and writes, assert no races)
  - Run with `go test -race`
- [ ] Test `GetLatest` for nonexistent token returns false

## Phase 6: REST API Handlers

- [ ] Implement handlers (`internal/api/handler.go`)
  - `GET /prices` — returns all latest prices (JSON array)
  - `GET /prices/{token}` — returns latest price for a token (JSON object), 404 if not found
  - `GET /prices/{token}/history?limit=N` — returns price history, default limit 60
- [ ] Implement router setup function that takes a `*store.Store` and returns `http.Handler`
- [ ] Add structured JSON error responses (consistent `{"error": "message"}` format)
- [ ] Add request logging middleware (method, path, status, duration)

## Phase 7: Tests — API Handlers

- [ ] Test `GET /prices` — empty store returns `[]`, populated store returns all tokens
- [ ] Test `GET /prices/{token}` — valid token, missing token (404)
- [ ] Test `GET /prices/{token}/history` — default limit, custom limit, invalid limit param
- [ ] Test JSON response structure matches expected schema
- [ ] Test middleware logs correctly (optional, lower priority)
- [ ] All tests use `httptest.NewRecorder` + a pre-populated store (no real Pyth calls)

## Phase 8: SSE Streaming

- [ ] Implement SSE endpoint (`internal/api/sse.go`)
  - `GET /events` — streams price updates as SSE events
  - `GET /events?tokens=BTC,ETH` — optional filter by token IDs
- [ ] Implement broker/hub pattern:
  - `Broker` struct manages subscriber channels
  - `Subscribe() (<-chan Price, unsubscribe func)`
  - `Publish(Price)` — fans out to all subscribers
- [ ] Wire broker: fetcher publishes to broker, SSE handler subscribes on connection
- [ ] Handle client disconnect (context cancellation), clean up subscriber

## Phase 9: Tests — SSE

- [ ] Test broker subscribe/publish/unsubscribe lifecycle
- [ ] Test broker fan-out to multiple subscribers
- [ ] Test subscriber cleanup on unsubscribe (no goroutine leak)
- [ ] Test SSE endpoint format — verify `data:` prefix, newline framing, `Content-Type: text/event-stream`
- [ ] Test token filter — subscriber only receives events for requested tokens
- [ ] Test client disconnect triggers cleanup

## Phase 10: Background Fetcher

- [ ] Implement fetcher (`internal/fetcher/fetcher.go`)
  - Struct holding pyth client, store, broker, config
  - `Run(ctx)` — starts ticker, fetches all configured tokens each tick
  - On each tick: for each token, call pyth client, convert to `Price`, write to store, publish to broker
  - Log errors per-token but continue with other tokens
- [ ] Handle graceful shutdown via context cancellation
- [ ] Stablecoin handling — skip API call, insert fixed price (like the Python repo)

## Phase 11: Tests — Fetcher

- [ ] Test fetcher calls pyth client for each configured token
- [ ] Test fetcher writes results to store
- [ ] Test fetcher publishes to broker
- [ ] Test fetcher skips stablecoins and inserts fixed price
- [ ] Test fetcher handles per-token API errors gracefully (other tokens still fetched)
- [ ] Test fetcher stops on context cancellation
- [ ] Use mock/fake pyth client (interface) and real store + broker

## Phase 12: Main Wiring & Server Startup

- [ ] Wire everything in `cmd/server/main.go`:
  - Load config
  - Create pyth client, store, broker, fetcher
  - Create router with handlers
  - Start fetcher in a goroutine
  - Start HTTP server
- [ ] Implement graceful shutdown:
  - Listen for SIGINT/SIGTERM
  - Cancel fetcher context
  - Call `http.Server.Shutdown` with timeout
- [ ] Add health check endpoint `GET /health`

## Phase 13: Integration Tests

- [ ] End-to-end test: start server (with mock pyth), verify:
  - `/health` returns 200
  - After one fetch cycle, `/prices` returns data
  - `/prices/{token}` returns correct price
  - `/prices/{token}/history` accumulates entries
  - `/events` SSE stream receives updates
- [ ] Race condition test: run full server with `-race` flag under load

## Phase 14: Polish

- [ ] Add `README.md` with setup instructions, endpoints, config reference
- [ ] Add `Makefile` or `Justfile` (build, test, run, lint)
- [ ] Add `.gitignore`
- [ ] Review error handling and edge cases
- [ ] Ensure all log output is structured and useful
