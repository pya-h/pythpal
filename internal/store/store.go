package store

import (
	"sync"
	"time"
)

type Store struct {
	mu         sync.RWMutex
	latest     map[string]Price
	history    map[string][]Price
	historyCap time.Duration // max age of history entries
}

func New(historyCapMinutes int) *Store {
	return &Store{
		latest:     make(map[string]Price),
		history:    make(map[string][]Price),
		historyCap: time.Duration(historyCapMinutes) * time.Minute,
	}
}

// SetLatest stores a new price, appends to history, and prunes old entries.
func (s *Store) SetLatest(tokenID string, p Price) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.latest[tokenID] = p
	s.history[tokenID] = append(s.history[tokenID], p)
	s.prune(tokenID)
}

// GetLatest returns the most recent price for a token.
func (s *Store) GetLatest(tokenID string) (Price, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.latest[tokenID]
	return p, ok
}

// GetAllLatest returns the latest price for every token.
func (s *Store) GetAllLatest() map[string]Price {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]Price, len(s.latest))
	for k, v := range s.latest {
		result[k] = v
	}
	return result
}

// GetHistory returns the last N price entries for a token (most recent last).
func (s *Store) GetHistory(tokenID string, limit int) []Price {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h := s.history[tokenID]
	if len(h) == 0 {
		return nil
	}

	if limit <= 0 || limit > len(h) {
		limit = len(h)
	}

	start := len(h) - limit
	result := make([]Price, limit)
	copy(result, h[start:])
	return result
}

// prune removes history entries older than the history cap.
// Must be called with s.mu held.
func (s *Store) prune(tokenID string) {
	h := s.history[tokenID]
	if len(h) == 0 {
		return
	}

	cutoff := time.Now().Add(-s.historyCap)
	i := 0
	for i < len(h) && h[i].Timestamp.Before(cutoff) {
		i++
	}
	if i > 0 {
		s.history[tokenID] = h[i:]
	}
}
