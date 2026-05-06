package api

import (
	"sync"

	"github.com/p4ya/umbra/pricefeed/internal/store"
)

type Broker struct {
	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
}

type subscriber struct {
	ch     chan store.Price
	tokens map[string]bool // nil means all tokens
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[*subscriber]struct{}),
	}
}

// Subscribe registers a new subscriber. If tokens is nil or empty, all prices
// are received. Returns a channel to read from and an unsubscribe function.
func (b *Broker) Subscribe(tokens []string) (<-chan store.Price, func()) {
	sub := &subscriber{
		ch: make(chan store.Price, 64),
	}
	if len(tokens) > 0 {
		sub.tokens = make(map[string]bool, len(tokens))
		for _, t := range tokens {
			sub.tokens[t] = true
		}
	}

	b.mu.Lock()
	b.subscribers[sub] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		delete(b.subscribers, sub)
		close(sub.ch)
		b.mu.Unlock()
	}

	return sub.ch, unsubscribe
}

// Publish sends a price update to all matching subscribers.
// Non-blocking: if a subscriber's buffer is full, the event is dropped.
func (b *Broker) Publish(p store.Price) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subscribers {
		if sub.tokens != nil && !sub.tokens[p.TokenID] {
			continue
		}
		select {
		case sub.ch <- p:
		default:
			// subscriber too slow, drop event
		}
	}
}

// Close closes all subscriber channels. Call on shutdown.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subscribers {
		close(sub.ch)
		delete(b.subscribers, sub)
	}
}
