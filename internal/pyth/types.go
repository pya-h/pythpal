package pyth

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/p4ya/umbra/priceservice/internal/store"
)

type HermesResponse struct {
	Parsed []ParsedPriceFeed `json:"parsed"`
}

type ParsedPriceFeed struct {
	ID       string    `json:"id"`
	Price    PriceData `json:"price"`
	EMAPrice PriceData `json:"ema_price"`
}

type PriceData struct {
	Price       string `json:"price"`
	Conf        string `json:"conf"`
	Expo        int    `json:"expo"`
	PublishTime int64  `json:"publish_time"`
}

// PriceFeedInfo represents a single feed entry from /v2/price_feeds.
type PriceFeedInfo struct {
	ID         string         `json:"id"`
	Attributes PriceFeedAttrs `json:"attributes"`
}

type PriceFeedAttrs struct {
	AssetType     string `json:"asset_type"`
	Base          string `json:"base"`
	QuoteCurrency string `json:"quote_currency"`
	Symbol        string `json:"symbol"`
}

// NormalizeName returns a clean, URL-safe token ID.
// USD pairs use just the base: "BTC". Non-USD pairs use "BTC-EUR".
// Falls back to parsing the symbol field if attributes are missing.
func (f *PriceFeedInfo) NormalizeName() string {
	if f.Attributes.Base != "" && f.Attributes.QuoteCurrency != "" {
		if f.Attributes.QuoteCurrency == "USD" {
			return f.Attributes.Base
		}
		return f.Attributes.Base + "-" + f.Attributes.QuoteCurrency
	}
	s := f.Attributes.Symbol
	if idx := strings.Index(s, "."); idx != -1 {
		s = s[idx+1:]
	}
	return strings.ReplaceAll(s, "/", "-")
}

// NormalizePrice converts a raw price string and exponent to a float64.
// e.g. price="236722004851", expo=-8 -> 2367.22004851
func NormalizePrice(rawPrice string, expo int) (float64, error) {
	val, err := strconv.ParseInt(rawPrice, 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(val) * math.Pow10(expo), nil
}

// ToStorePrice converts a ParsedPriceFeed to a store.Price for a given tokenID.
func (p *ParsedPriceFeed) ToStorePrice(tokenID string) (store.Price, error) {
	price, err := NormalizePrice(p.Price.Price, p.Price.Expo)
	if err != nil {
		return store.Price{}, err
	}

	emaPrice, err := NormalizePrice(p.EMAPrice.Price, p.EMAPrice.Expo)
	if err != nil {
		return store.Price{}, err
	}

	conf, err := NormalizePrice(p.Price.Conf, p.Price.Expo)
	if err != nil {
		return store.Price{}, err
	}

	return store.Price{
		TokenID:   tokenID,
		Price:     price,
		EMAPrice:  emaPrice,
		Conf:      conf,
		Timestamp: time.Unix(p.Price.PublishTime, 0),
	}, nil
}
