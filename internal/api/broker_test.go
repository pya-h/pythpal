package api

import (
	"testing"
	"time"

	"github.com/p4ya/umbra/pricefeed/internal/store"
)

func makeTestPrice(token string, price float64) store.Price {
	return store.Price{
		TokenID:   token,
		Price:     price,
		Timestamp: time.Now(),
	}
}

func TestBroker_SubscribePublish(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch, unsub := b.Subscribe(nil)
	defer unsub()

	p := makeTestPrice("BTC", 94500)
	b.Publish(p)

	select {
	case got := <-ch:
		if got.TokenID != "BTC" || got.Price != 94500 {
			t.Errorf("got %+v, want BTC@94500", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published price")
	}
}

func TestBroker_FanOut(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch1, unsub1 := b.Subscribe(nil)
	defer unsub1()
	ch2, unsub2 := b.Subscribe(nil)
	defer unsub2()

	p := makeTestPrice("ETH", 2400)
	b.Publish(p)

	for i, ch := range []<-chan store.Price{ch1, ch2} {
		select {
		case got := <-ch:
			if got.TokenID != "ETH" {
				t.Errorf("subscriber %d: got %q, want ETH", i, got.TokenID)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestBroker_Unsubscribe(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch, unsub := b.Subscribe(nil)

	// Unsubscribe closes the channel
	unsub()

	// Publish should not panic or block
	b.Publish(makeTestPrice("BTC", 90000))

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}
}

func TestBroker_TokenFilter(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch, unsub := b.Subscribe([]string{"ETH"})
	defer unsub()

	b.Publish(makeTestPrice("BTC", 94500)) // should be filtered out
	b.Publish(makeTestPrice("ETH", 2400))  // should come through

	select {
	case got := <-ch:
		if got.TokenID != "ETH" {
			t.Errorf("got %q, want ETH (BTC should be filtered)", got.TokenID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ETH price")
	}

	// Verify BTC did NOT come through
	select {
	case got := <-ch:
		t.Errorf("unexpected price received: %+v", got)
	case <-time.After(50 * time.Millisecond):
		// good — nothing else in channel
	}
}

func TestBroker_MultipleTokenFilter(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch, unsub := b.Subscribe([]string{"BTC", "ETH"})
	defer unsub()

	b.Publish(makeTestPrice("SOL", 150))   // filtered
	b.Publish(makeTestPrice("BTC", 94500)) // pass
	b.Publish(makeTestPrice("ETH", 2400))  // pass

	received := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case got := <-ch:
			received[got.TokenID] = true
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
	}

	if !received["BTC"] || !received["ETH"] {
		t.Errorf("expected BTC and ETH, got %v", received)
	}
}

func TestBroker_NoSubscribers(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	// Should not panic
	b.Publish(makeTestPrice("BTC", 94500))
}

func TestBroker_SlowSubscriberDropsEvents(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch, unsub := b.Subscribe(nil)
	defer unsub()

	// Fill the buffer (capacity 64) + extra
	for i := 0; i < 80; i++ {
		b.Publish(makeTestPrice("BTC", float64(i)))
	}

	// Should get at most 64 events (buffer size)
	count := 0
	for {
		select {
		case <-ch:
			count++
		case <-time.After(50 * time.Millisecond):
			if count != 64 {
				t.Errorf("got %d events, want 64 (buffer capacity)", count)
			}
			return
		}
	}
}
