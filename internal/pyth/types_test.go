package pyth

import (
	"math"
	"testing"
)

func TestNormalizePrice(t *testing.T) {
	tests := []struct {
		name     string
		rawPrice string
		expo     int
		want     float64
		wantErr  bool
	}{
		{
			name:     "ETH typical price",
			rawPrice: "236722004851",
			expo:     -8,
			want:     2367.22004851,
		},
		{
			name:     "BTC typical price",
			rawPrice: "9450000000000",
			expo:     -8,
			want:     94500.0,
		},
		{
			name:     "zero price",
			rawPrice: "0",
			expo:     -8,
			want:     0,
		},
		{
			name:     "positive exponent",
			rawPrice: "100",
			expo:     2,
			want:     10000,
		},
		{
			name:     "invalid price string",
			rawPrice: "notanumber",
			expo:     -8,
			wantErr:  true,
		},
		{
			name:     "empty price string",
			rawPrice: "",
			expo:     -8,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePrice(tt.rawPrice, tt.expo)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

func TestParsedPriceFeed_ToStorePrice(t *testing.T) {
	feed := ParsedPriceFeed{
		ID: "ff61491a931112ddf1bd8147cd1b641375f79f5825126d665480874634fd0ace",
		Price: PriceData{
			Price:       "236722004851",
			Conf:        "138860300",
			Expo:        -8,
			PublishTime: 1700000000,
		},
		EMAPrice: PriceData{
			Price:       "239686652000",
			Conf:        "97739563",
			Expo:        -8,
			PublishTime: 1700000000,
		},
	}

	sp, err := feed.ToStorePrice("ETH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp.TokenID != "ETH" {
		t.Errorf("token_id = %q, want %q", sp.TokenID, "ETH")
	}
	if math.Abs(sp.Price-2367.22004851) > 1e-6 {
		t.Errorf("price = %f, want ~2367.22", sp.Price)
	}
	if math.Abs(sp.EMAPrice-2396.86652) > 1e-4 {
		t.Errorf("ema_price = %f, want ~2396.87", sp.EMAPrice)
	}
	if sp.Conf <= 0 {
		t.Errorf("confidence should be positive, got %f", sp.Conf)
	}
	if sp.Timestamp.Unix() != 1700000000 {
		t.Errorf("timestamp = %d, want 1700000000", sp.Timestamp.Unix())
	}
}

func TestParsedPriceFeed_ToStorePrice_InvalidPrice(t *testing.T) {
	feed := ParsedPriceFeed{
		Price: PriceData{
			Price: "invalid",
			Expo:  -8,
		},
		EMAPrice: PriceData{
			Price: "100",
			Expo:  -8,
		},
	}

	_, err := feed.ToStorePrice("BAD")
	if err == nil {
		t.Fatal("expected error for invalid price, got nil")
	}
}
