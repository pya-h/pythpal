# Price Feed Service — Integration Guide

Base URL: `{PRICESERVICE_URL}` (e.g. `https://priceservice.example.com`)

## Core Endpoint

### `GET /prices/{token}`

Returns the current price for a single token.

**Path param:** `token` — the token ID (e.g. `BTC`, `ETH`, `SOL`, `USDT`)

**Response (200):**

```json
{
  "token_id": "BTC",
  "price": 94500.12,
  "ema_price": 94600.50,
  "confidence": 0.001,
  "timestamp": "2025-01-15T12:00:05Z",
  "stale": false
}
```

| Field | Type | Description |
|---|---|---|
| `token_id` | `string` | Token identifier, matches the path param |
| `price` | `float64` | Current spot price in USD |
| `ema_price` | `float64` | Exponential moving average price |
| `confidence` | `float64` | Price confidence interval (lower = more certain) |
| `timestamp` | `string` (RFC 3339) | When Pyth published this price |
| `stale` | `bool` | `true` if the price hasn't been updated in over 2 minutes. Omitted (absent) when fresh. |

**Response (404):**

```json
{
  "error": "token not found"
}
```

**Usage notes:**
- Token IDs are uppercase: `BTC`, `ETH`, `SOL`, not `btc` or `Bitcoin`
- Prices update every ~60 seconds
- Check `stale` field — if present and `true`, the price may be outdated (service couldn't reach Pyth)
- `timestamp` is always UTC in RFC 3339 format

---

## Secondary Endpoints

### `GET /prices?tokens=BTC,ETH,SOL`

Fetch multiple token prices in a single call.

**Query param:** `tokens` — comma-separated list of token IDs

**Response (200):** JSON array of price objects (same shape as above). Tokens not found are silently omitted from the array.

```json
[
  {"token_id": "BTC", "price": 94500.12, ...},
  {"token_id": "ETH", "price": 2367.22, ...}
]
```

### `GET /health`

**Response (200):**

```json
{"status": "ok"}
```

Use for connectivity checks / readiness probes.

---

## Real-Time Streaming (SSE)

### `GET /events?tokens=BTC,ETH`

Server-Sent Events stream. Pushes a new event every time a token's price updates (~every 60s per token).

**Query param:** `tokens` (optional) — comma-separated filter. Omit to receive all tokens.

**Connection:** Standard SSE — set `Accept: text/event-stream` or just use an SSE client library. The connection stays open indefinitely.

**Event format:**

```
data: {"token_id":"BTC","price":94500.12,"ema_price":94600.50,"confidence":0.001,"timestamp":"2025-01-15T12:00:05Z"}

data: {"token_id":"ETH","price":2367.22,"ema_price":2396.87,"confidence":0.0014,"timestamp":"2025-01-15T12:00:05Z"}

```

Each event is a single `data:` line followed by a blank line. Parse the JSON after `data: `.

**Client examples:**

```javascript
// JavaScript / Node.js
const es = new EventSource(`${PRICESERVICE_URL}/events?tokens=BTC,ETH`);
es.onmessage = (e) => {
  const price = JSON.parse(e.data);
  console.log(price.token_id, price.price);
};
```

```go
// Go — using standard net/http
resp, _ := http.Get(priceserviceURL + "/events?tokens=BTC")
scanner := bufio.NewScanner(resp.Body)
for scanner.Scan() {
    line := scanner.Text()
    if strings.HasPrefix(line, "data: ") {
        var p PriceEvent
        json.Unmarshal([]byte(line[6:]), &p)
        // use p.Price
    }
}
```

**Reconnection:** If the connection drops, simply reconnect. The service only streams live data (no replay), so you'll get the next update on the next fetch cycle.
