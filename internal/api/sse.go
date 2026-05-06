package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// RegisterSSE registers the SSE endpoint on the mux.
// Separated from Handler because SSE depends on the Broker, not just the Store.
func RegisterSSE(mux *http.ServeMux, broker *Broker) {
	mux.Handle("GET /events", logMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		var tokens []string
		if q := r.URL.Query().Get("tokens"); q != "" {
			for _, t := range strings.Split(q, ",") {
				if t = strings.TrimSpace(t); t != "" {
					tokens = append(tokens, t)
				}
			}
		}

		ch, unsubscribe := broker.Subscribe(tokens)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				unsubscribe()
				slog.Info("sse client disconnected", "remote", r.RemoteAddr)
				return
			case price, ok := <-ch:
				if !ok {
					return
				}
				data, err := json.Marshal(price)
				if err != nil {
					slog.Error("sse marshal error", "error", err)
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	})))
}
