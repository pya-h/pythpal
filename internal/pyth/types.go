package pyth

import (
	"math"
	"strconv"
	"time"

	"github.com/p4ya/umbra/pricefeed/internal/store"
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
