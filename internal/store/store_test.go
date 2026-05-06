package store

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func makePrice(tokenID string, price float64, age time.Duration) Price {
	return Price{
		TokenID:   tokenID,
		Price:     price,
		EMAPrice:  price * 1.01,
		Conf:      0.5,
		Timestamp: time.Now().Add(-age),
	}
}

func TestSetLatest_GetLatest(t *testing.T) {
	s := New(1440)
	p := makePrice("BTC", 94500, 0)

	s.SetLatest("BTC", p)
	got, ok := s.GetLatest("BTC")
	if !ok {
		t.Fatal("expected to find BTC price")
	}
	if got.Price != 94500 {
		t.Errorf("price = %f, want 94500", got.Price)
	}
}

func TestGetLatest_NotFound(t *testing.T) {
	s := New(1440)
	_, ok := s.GetLatest("MISSING")
	if ok {
		t.Fatal("expected not found for nonexistent token")
	}
}

func TestSetLatest_Overwrites(t *testing.T) {
	s := New(1440)
	s.SetLatest("ETH", makePrice("ETH", 2300, 0))
	s.SetLatest("ETH", makePrice("ETH", 2400, 0))

	got, _ := s.GetLatest("ETH")
	if got.Price != 2400 {
		t.Errorf("price = %f, want 2400 (latest)", got.Price)
	}
}

func TestGetAllLatest(t *testing.T) {
	s := New(1440)
	s.SetLatest("BTC", makePrice("BTC", 94500, 0))
	s.SetLatest("ETH", makePrice("ETH", 2400, 0))
	s.SetLatest("SOL", makePrice("SOL", 150, 0))

	all := s.GetAllLatest()
	if len(all) != 3 {
		t.Fatalf("got %d tokens, want 3", len(all))
	}
	if all["BTC"].Price != 94500 {
		t.Errorf("BTC = %f, want 94500", all["BTC"].Price)
	}
	if all["ETH"].Price != 2400 {
		t.Errorf("ETH = %f, want 2400", all["ETH"].Price)
	}
}

func TestGetHistory_Order(t *testing.T) {
	s := New(1440)
	for i := 0; i < 5; i++ {
		p := Price{
			TokenID:   "BTC",
			Price:     float64(90000 + i*1000),
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
		}
		s.SetLatest("BTC", p)
	}

	h := s.GetHistory("BTC", 3)
	if len(h) != 3 {
		t.Fatalf("got %d entries, want 3", len(h))
	}
	// Should be ordered oldest to newest (last 3)
	if h[0].Price != 92000 {
		t.Errorf("first = %f, want 92000", h[0].Price)
	}
	if h[2].Price != 94000 {
		t.Errorf("last = %f, want 94000", h[2].Price)
	}
}

func TestGetHistory_LimitExceedsSize(t *testing.T) {
	s := New(1440)
	s.SetLatest("ETH", makePrice("ETH", 2400, 0))
	s.SetLatest("ETH", makePrice("ETH", 2500, 0))

	h := s.GetHistory("ETH", 100)
	if len(h) != 2 {
		t.Errorf("got %d entries, want 2 (all available)", len(h))
	}
}

func TestGetHistory_Empty(t *testing.T) {
	s := New(1440)
	h := s.GetHistory("NONE", 10)
	if h != nil {
		t.Errorf("expected nil for nonexistent token, got %v", h)
	}
}

func TestHistoryCap_PrunesOldEntries(t *testing.T) {
	s := New(5) // 5 minutes cap

	// Insert an entry from 10 minutes ago
	old := Price{
		TokenID:   "BTC",
		Price:     90000,
		Timestamp: time.Now().Add(-10 * time.Minute),
	}
	s.SetLatest("BTC", old)

	// Insert a fresh entry — this triggers pruning
	fresh := makePrice("BTC", 95000, 0)
	s.SetLatest("BTC", fresh)

	h := s.GetHistory("BTC", 100)
	if len(h) != 1 {
		t.Fatalf("got %d entries, want 1 (old should be pruned)", len(h))
	}
	if h[0].Price != 95000 {
		t.Errorf("remaining price = %f, want 95000", h[0].Price)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	s := New(1440)
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				token := fmt.Sprintf("TOKEN%d", i)
				s.SetLatest(token, makePrice(token, float64(j), 0))
			}
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				token := fmt.Sprintf("TOKEN%d", i)
				s.GetLatest(token)
				s.GetAllLatest()
				s.GetHistory(token, 10)
			}
		}(i)
	}

	wg.Wait()
}
