// Package observe provides the control-plane HTTP handler for the observe query facade.
package observe

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// BackendConfig holds URLs for the observe backends, read from env at startup.
// Kept for backwards-compat; multi-region path uses LoadBackends instead.
type BackendConfig struct {
	VictoriaMetricsURL string
	LokiURL            string
	TempoURL           string
}

// Handler is a chi-compatible HTTP handler for the unified observe facade.
type Handler struct {
	db     *sql.DB
	client *MultiClient
}

// NewHandler creates a Handler backed by the DB-loaded (multi-region) MultiClient.
func NewHandler(db *sql.DB) *Handler {
	backends, _ := LoadBackends(db)
	return &Handler{
		db:     db,
		client: NewMultiClient(backends),
	}
}

// newHandlerFromConfig creates a Handler from an explicit BackendConfig (test / legacy path).
func newHandlerFromConfig(cfg BackendConfig) *Handler {
	backends := []RegionBackend{}
	if cfg.VictoriaMetricsURL != "" || cfg.LokiURL != "" || cfg.TempoURL != "" {
		backends = []RegionBackend{{
			Region:   "default",
			VMURL:    cfg.VictoriaMetricsURL,
			LokiURL:  cfg.LokiURL,
			TempoURL: cfg.TempoURL,
		}}
	}
	return &Handler{client: NewMultiClient(backends)}
}

// QueryMetrics handles GET /v1/observe/metrics?query=<promql>&region=<optional>
func (h *Handler) QueryMetrics(w http.ResponseWriter, r *http.Request) {
	if len(h.client.backends) == 0 {
		writeJSON(w, http.StatusOK, notConfigured("metrics"))
		return
	}
	promql := r.URL.Query().Get("query")
	if promql == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter required"})
		return
	}
	region := r.URL.Query().Get("region")
	rp, err := parseRange(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	results, err := h.client.QueryMetricsRange(r.Context(), promql, region, rp)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// parseRange reads start/end (unix seconds) and step ("60s" or seconds).
// No step means an instant query.
func parseRange(q url.Values) (RangeParams, error) {
	step := q.Get("step")
	if step == "" {
		return RangeParams{}, nil
	}
	if _, err := strconv.ParseFloat(step, 64); err != nil {
		if _, err := time.ParseDuration(step); err != nil {
			return RangeParams{}, fmt.Errorf("invalid step %q", step)
		}
	}
	rp := RangeParams{End: time.Now(), Step: step}
	if e := q.Get("end"); e != "" {
		n, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return RangeParams{}, fmt.Errorf("invalid end %q", e)
		}
		rp.End = time.Unix(n, 0)
	}
	rp.Start = rp.End.Add(-time.Hour)
	if s := q.Get("start"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return RangeParams{}, fmt.Errorf("invalid start %q", s)
		}
		rp.Start = time.Unix(n, 0)
	}
	return rp, nil
}

// QueryLogs handles GET /v1/observe/logs?query=<logql>&start=<>&end=<>&region=<optional>
func (h *Handler) QueryLogs(w http.ResponseWriter, r *http.Request) {
	if len(h.client.backends) == 0 {
		writeJSON(w, http.StatusOK, notConfigured("logs"))
		return
	}
	q := r.URL.Query()
	logql := q.Get("query")
	if logql == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter required"})
		return
	}
	now := time.Now()
	start, end := now.Add(-1*time.Hour), now
	if s := q.Get("start"); s != "" {
		if ns, err := parseTime(s); err == nil {
			start = ns
		}
	}
	if e := q.Get("end"); e != "" {
		if ns, err := parseTime(e); err == nil {
			end = ns
		}
	}
	region := q.Get("region")
	results, err := h.client.QueryLogs(r.Context(), logql, region, start, end)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// QueryTrace handles GET /v1/observe/traces/{traceID}?region=<optional>
func (h *Handler) QueryTrace(w http.ResponseWriter, r *http.Request) {
	if len(h.client.backends) == 0 {
		writeJSON(w, http.StatusOK, notConfigured("traces"))
		return
	}
	traceID := chi.URLParam(r, "traceID")
	if traceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "traceID required"})
		return
	}
	region := r.URL.Query().Get("region")
	data, err := h.client.QueryTrace(r.Context(), traceID, region)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func notConfigured(kind string) map[string]string {
	return map[string]string{
		"status":  "backends_not_configured",
		"message": fmt.Sprintf("Set vm_url/loki_url/tempo_url on your regions, or BORDO_VM_URL/BORDO_LOKI_URL/BORDO_TEMPO_URL env vars to enable %s queries", kind),
	}
}

// parseTime parses a Unix nanosecond timestamp string.
func parseTime(s string) (time.Time, error) {
	var ns int64
	_, err := fmt.Sscanf(s, "%d", &ns)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, ns), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
