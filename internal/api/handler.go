package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/p4ya/umbra/pricefeed/internal/store"
)

type Handler struct {
	store *store.Store
}

func NewHandler(s *store.Store) *Handler {
	return &Handler{store: s}
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
	all := h.store.GetAllLatest()
	prices := make([]store.Price, 0, len(all))
	for _, p := range all {
		prices = append(prices, p)
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
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).String(),
		)
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
