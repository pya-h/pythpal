package pyth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// PriceFetcher is the interface used by the fetcher to get prices.
type PriceFetcher interface {
	FetchLatestPrices(ctx context.Context, feedIDs []string) ([]ParsedPriceFeed, error)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListPriceFeeds discovers all available price feeds from Hermes.
func (c *Client) ListPriceFeeds(ctx context.Context) ([]PriceFeedInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v2/price_feeds", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hermes API returned status %d", resp.StatusCode)
	}

	var feeds []PriceFeedInfo
	if err := json.NewDecoder(resp.Body).Decode(&feeds); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return feeds, nil
}

// FetchLatestPrices batch-fetches the latest prices for multiple feed IDs.
// Chunks requests to avoid URL length limits.
func (c *Client) FetchLatestPrices(ctx context.Context, feedIDs []string) ([]ParsedPriceFeed, error) {
	const chunkSize = 100

	var all []ParsedPriceFeed
	totalChunks := (len(feedIDs) + chunkSize - 1) / chunkSize

	for i := 0; i < len(feedIDs); i += chunkSize {
		if ctx.Err() != nil {
			return all, ctx.Err()
		}

		end := i + chunkSize
		if end > len(feedIDs) {
			end = len(feedIDs)
		}

		chunkNum := i/chunkSize + 1
		chunk, err := c.fetchBatch(ctx, feedIDs[i:end])
		if err != nil {
			slog.Warn("chunk fetch failed", "chunk", chunkNum, "total_chunks", totalChunks, "error", err)
			continue
		}
		all = append(all, chunk...)
		slog.Debug("chunk fetched", "chunk", chunkNum, "total_chunks", totalChunks, "prices_so_far", len(all))
	}

	return all, nil
}

// FetchLatestPrice fetches the latest price for a single feed ID.
func (c *Client) FetchLatestPrice(ctx context.Context, feedID string) (*ParsedPriceFeed, error) {
	results, err := c.FetchLatestPrices(ctx, []string{feedID})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no price data returned for feed")
	}
	return &results[0], nil
}

// FetchPriceAtTime fetches the price at a specific timestamp for a given feed ID.
func (c *Client) FetchPriceAtTime(ctx context.Context, feedID string, timestamp time.Time) (*ParsedPriceFeed, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v2/updates/price/%d", c.baseURL, timestamp.Unix()))
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	q := u.Query()
	q.Set("ids[]", feedID)
	q.Set("parsed", "true")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hermes API returned status %d", resp.StatusCode)
	}

	var hermesResp HermesResponse
	if err := json.NewDecoder(resp.Body).Decode(&hermesResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(hermesResp.Parsed) == 0 {
		return nil, fmt.Errorf("no price data returned for feed")
	}

	return &hermesResp.Parsed[0], nil
}

func (c *Client) fetchBatch(ctx context.Context, feedIDs []string) ([]ParsedPriceFeed, error) {
	u, err := url.Parse(c.baseURL + "/v2/updates/price/latest")
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}

	q := u.Query()
	for _, id := range feedIDs {
		q.Add("ids[]", id)
	}
	q.Set("parsed", "true")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hermes API returned status %d", resp.StatusCode)
	}

	var hermesResp HermesResponse
	if err := json.NewDecoder(resp.Body).Decode(&hermesResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return hermesResp.Parsed, nil
}
