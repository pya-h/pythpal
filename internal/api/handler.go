package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/p4ya/umbra/pricefeed/internal/store"
)

type Handler struct {
	store          *store.Store
	staleThreshold time.Duration
}

func NewHandler(s *store.Store, staleThreshold time.Duration) *Handler {
	return &Handler{store: s, staleThreshold: staleThreshold}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /prices", logMiddleware(http.HandlerFunc(h.handleGetAllPrices)))
	mux.Handle("GET /prices/{token}", logMiddleware(http.HandlerFunc(h.handleGetPrice)))
	mux.Handle("GET /prices/{token}/history", logMiddleware(http.HandlerFunc(h.handleGetHistory)))
	mux.Handle("GET /health", logMiddleware(http.HandlerFunc(h.handleHealth)))
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleGetAllPrices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tokensParam := q.Get("tokens")
	skipParam := q.Get("skip")
	takeParam := q.Get("take")

	hasFilter := tokensParam != ""
	hasPagination := skipParam != "" || takeParam != ""

	if hasFilter && hasPagination {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "cannot use 'tokens' filter together with pagination ('skip'/'take')",
		})
		return
	}

	all := h.store.GetAllLatest()
	now := time.Now()

	// Apply tokens filter
	if hasFilter {
		ids := strings.Split(tokensParam, ",")
		prices := make([]store.Price, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if p, ok := all[id]; ok {
				p.Stale = now.Sub(p.Timestamp) > h.staleThreshold
				prices = append(prices, p)
			}
		}
		writeJSON(w, http.StatusOK, prices)
		return
	}

	// Build full list
	prices := make([]store.Price, 0, len(all))
	for _, p := range all {
		p.Stale = now.Sub(p.Timestamp) > h.staleThreshold
		prices = append(prices, p)
	}

	// Apply pagination
	if hasPagination {
		skip := 0
		take := len(prices)

		if skipParam != "" {
			var err error
			skip, err = strconv.Atoi(skipParam)
			if err != nil || skip < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'skip' parameter"})
				return
			}
		}
		if takeParam != "" {
			var err error
			take, err = strconv.Atoi(takeParam)
			if err != nil || take < 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'take' parameter"})
				return
			}
		}

		if skip >= len(prices) {
			prices = []store.Price{}
		} else {
			end := skip + take
			if end > len(prices) {
				end = len(prices)
			}
			prices = prices[skip:end]
		}
	}

	writeJSON(w, http.StatusOK, prices)
}

func (h *Handler) handleGetPrice(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	p, ok := h.store.GetLatest(token)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
		return
	}
	p.Stale = time.Since(p.Timestamp) > h.staleThreshold
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	limitStr := r.URL.Query().Get("limit")
	limit := 60
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit parameter"})
			return
		}
	}

	history := h.store.GetHistory(token, limit)
	if history == nil {
		history = []store.Price{}
	}
	writeJSON(w, http.StatusOK, history)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).String(),
		}

		switch {
		case rec.status >= 500:
			slog.Error("request", attrs...)
		case rec.status >= 400:
			slog.Warn("request", attrs...)
		default:
			slog.Debug("request", attrs...)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
