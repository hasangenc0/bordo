// Package observe provides the control-plane HTTP handler for the observe query facade.
package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
)

// BackendConfig holds URLs for the observe backends, read from env at startup.
type BackendConfig struct {
	VictoriaMetricsURL string
	LokiURL            string
	TempoURL           string
}

// Handler is a chi-compatible HTTP handler for observe queries.
type Handler struct {
	cfg  BackendConfig
	http *http.Client
}

// NewHandler creates a Handler.
func NewHandler(cfg BackendConfig) *Handler {
	return &Handler{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// QueryMetrics handles GET /v1/observe/metrics?query=<promql>
func (h *Handler) QueryMetrics(w http.ResponseWriter, r *http.Request) {
	if h.cfg.VictoriaMetricsURL == "" {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "backends_not_configured",
			"message": "Set BORDO_VM_URL to enable metrics queries",
		})
		return
	}
	promql := r.URL.Query().Get("query")
	if promql == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter required"})
		return
	}
	params := url.Values{"query": {promql}, "time": {fmt.Sprintf("%d", time.Now().Unix())}}
	h.proxy(r.Context(), w, h.cfg.VictoriaMetricsURL+"/api/v1/query?"+params.Encode())
}

// QueryLogs handles GET /v1/observe/logs?query=<logql>&start=<>&end=<>
func (h *Handler) QueryLogs(w http.ResponseWriter, r *http.Request) {
	if h.cfg.LokiURL == "" {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "backends_not_configured",
			"message": "Set BORDO_LOKI_URL to enable log queries",
		})
		return
	}
	q := r.URL.Query()
	logql := q.Get("query")
	if logql == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter required"})
		return
	}
	start := q.Get("start")
	end := q.Get("end")
	if start == "" {
		start = fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).UnixNano())
	}
	if end == "" {
		end = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	params := url.Values{"query": {logql}, "start": {start}, "end": {end}, "limit": {"100"}}
	h.proxy(r.Context(), w, h.cfg.LokiURL+"/loki/api/v1/query_range?"+params.Encode())
}

// QueryTrace handles GET /v1/observe/traces/{traceID}
func (h *Handler) QueryTrace(w http.ResponseWriter, r *http.Request) {
	if h.cfg.TempoURL == "" {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "backends_not_configured",
			"message": "Set BORDO_TEMPO_URL to enable trace queries",
		})
		return
	}
	traceID := chi.URLParam(r, "traceID")
	if traceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "traceID required"})
		return
	}
	h.proxy(r.Context(), w, h.cfg.TempoURL+"/api/traces/"+traceID)
}

// proxy forwards a GET request to upstream and streams the response body.
func (h *Handler) proxy(ctx context.Context, w http.ResponseWriter, upstream string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp, err := h.http.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
