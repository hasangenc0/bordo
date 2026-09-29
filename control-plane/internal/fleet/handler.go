package fleet

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type addRegionRequest struct {
	Name       string `json:"name"`
	Kubeconfig string `json:"kubeconfig"`
}

// NewHandler returns a chi router for the /regions REST API.
func NewHandler(store *Store, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	h := &handler{store: store, logger: logger}
	r.Post("/", h.add)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Delete("/{id}", h.remove)
	r.Post("/{name}/bootstrap", h.bootstrap)
	r.Post("/{name}/health", h.reportHealth)
	r.Get("/{name}/health", h.getHealth)
	return r
}

type handler struct {
	store  *Store
	logger *slog.Logger
}

// NodeHealth is a single node's reported health.
type NodeHealth struct {
	Name          string    `json:"name"`
	Ready         bool      `json:"ready"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

func (h *handler) reportHealth(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var report struct {
		Nodes []NodeHealth `json:"nodes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, n := range report.Nodes {
		id := uuid.NewString()
		ready := 0
		if n.Ready {
			ready = 1
		}
		_, err := h.store.db.ExecContext(r.Context(), `
			INSERT INTO bordo_node_health (id, region_name, node_name, ready, last_heartbeat, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(region_name, node_name) DO UPDATE SET
				ready = excluded.ready,
				last_heartbeat = excluded.last_heartbeat,
				updated_at = excluded.updated_at`,
			id, name, n.Name, ready, n.LastHeartbeat.UTC(), time.Now().UTC(),
		)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) getHealth(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	rows, err := h.store.db.QueryContext(r.Context(), `
		SELECT node_name, ready, last_heartbeat
		FROM bordo_node_health WHERE region_name = ? ORDER BY node_name`, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	var nodes []NodeHealth
	for rows.Next() {
		var n NodeHealth
		var ready int
		var heartbeat sql.NullTime
		if err := rows.Scan(&n.Name, &ready, &heartbeat); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		n.Ready = ready == 1
		if heartbeat.Valid {
			n.LastHeartbeat = heartbeat.Time
		}
		nodes = append(nodes, n)
	}
	if nodes == nil {
		nodes = []NodeHealth{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": name, "nodes": nodes})
}

func (h *handler) add(w http.ResponseWriter, r *http.Request) {
	var req addRegionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	region, err := h.store.AddRegion(r.Context(), req.Name, req.Kubeconfig)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/v1/regions/%s", region.ID))
	writeJSON(w, http.StatusCreated, region)
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	regions, err := h.store.ListRegions(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if regions == nil {
		regions = []*Region{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": regions})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	region, err := h.store.GetRegion(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "region not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, region)
}

func (h *handler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.store.RemoveRegion(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "region not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bootstrapRequest struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	User   string `json:"user"`
	SSHKey string `json:"ssh_key"`
}

func (h *handler) bootstrap(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req bootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Host == "" || req.SSHKey == "" {
		writeErr(w, http.StatusBadRequest, "host and ssh_key are required")
		return
	}

	// Ensure region exists; create if needed.
	if _, err := h.store.GetRegionByName(r.Context(), name); errors.Is(err, ErrNotFound) {
		if _, err2 := h.store.AddRegion(r.Context(), name, ""); err2 != nil {
			writeErr(w, http.StatusInternalServerError, "could not create region")
			return
		}
	}
	_ = h.store.UpdateRegionStatusByName(r.Context(), name, "bootstrapping")

	cfg := BootstrapConfig{
		Host:       req.Host,
		Port:       req.Port,
		User:       req.User,
		SSHKey:     req.SSHKey,
		RegionName: name,
	}

	go func() {
		ctx := context.Background()
		var logBuf bytes.Buffer
		kubeconfig, err := bootstrapK3s(ctx, cfg, &logBuf)
		if err != nil {
			h.logger.Error("bootstrap failed", "region", name, "err", err, "log", logBuf.String())
			_ = h.store.UpdateRegionStatusByName(ctx, name, "unreachable")
			return
		}
		if err := h.store.UpdateKubeconfig(ctx, name, kubeconfig, "healthy"); err != nil {
			h.logger.Error("bootstrap: failed to save kubeconfig", "region", name, "err", err)
			return
		}
		h.logger.Info("bootstrap complete", "region", name)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "bootstrapping", "region": name})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
