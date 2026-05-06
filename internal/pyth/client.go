package pyth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// PriceFetcher is the interface used by the fetcher to get prices.
type PriceFetcher interface {
	FetchLatestPrice(ctx context.Context, feedID string) (*ParsedPriceFeed, error)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchLatestPrice fetches the latest price for a given feed ID from Hermes.
func (c *Client) FetchLatestPrice(ctx context.Context, feedID string) (*ParsedPriceFeed, error) {
	u, err := url.Parse(c.baseURL + "/v2/updates/price/latest")
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	q := u.Query()
	q.Set("ids[]", feedID)
	q.Set("parsed", "true")
	u.RawQuery = q.Encode()

	return c.fetch(ctx, u.String())
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

	return c.fetch(ctx, u.String())
}

func (c *Client) fetch(ctx context.Context, rawURL string) (*ParsedPriceFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
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
