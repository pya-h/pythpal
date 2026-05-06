package store

import "time"

type Price struct {
	TokenID   string    `json:"token_id"`
	Price     float64   `json:"price"`
	EMAPrice  float64   `json:"ema_price"`
	Conf      float64   `json:"confidence"`
	Timestamp time.Time `json:"timestamp"`
}
