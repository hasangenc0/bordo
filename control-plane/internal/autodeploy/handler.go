package autodeploy

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handler exposes /v1/projects/{projectID}/autodeploy.
type Handler struct {
	db    *sql.DB
	store *Store
	// head reads the repository's current default-branch commit; it becomes the baseline when
	// auto-deploy is enabled so that code already live is not redeployed.
	head func(ctx context.Context, projectID string) (string, error)
}

func NewHandler(db *sql.DB, store *Store, head func(context.Context, string) (string, error)) http.Handler {
	h := &Handler{db: db, store: store, head: head}
	r := chi.NewRouter()
	r.Get("/", h.get)
	r.Put("/", h.set)
	r.Delete("/", h.disable)
	return r
}

func (h *Handler) project(w http.ResponseWriter, r *http.Request) (string, bool) {
	ref := chi.URLParam(r, "projectID")
	var id string
	err := h.db.QueryRowContext(r.Context(), `SELECT id FROM projects WHERE id = ? OR name = ? LIMIT 1`, ref, ref).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found (use the id or exact name from project_list)"})
		return "", false
	}
	return id, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.project(w, r)
	if !ok {
		return
	}
	c, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if c == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "config": c})
}

type setRequest struct {
	Region          string `json:"region"`
	Expose          string `json:"expose"`
	RequireApproval *bool  `json:"require_approval"`
}

func (h *Handler) set(w http.ResponseWriter, r *http.Request) {
	id, ok := h.project(w, r)
	if !ok {
		return
	}
	var req setRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Region == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "region is required"})
		return
	}
	var known int
	if err := h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM regions WHERE name = ?`, req.Region).Scan(&known); err != nil || known == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown region " + req.Region})
		return
	}
	switch req.Expose {
	case "", "none", "nodeport", "ingress":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expose must be none, nodeport or ingress"})
		return
	}
	// Deploying to a region without asking is opt-out, not the default.
	requireApproval := req.RequireApproval == nil || *req.RequireApproval
	baseline, err := h.head(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "cannot read the project's repository: " + err.Error()})
		return
	}
	c := Config{ProjectID: id, Region: req.Region, Expose: req.Expose, RequireApproval: requireApproval}
	if err := h.store.Set(r.Context(), c, baseline); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	cur, _ := h.store.Get(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "config": cur})
}

func (h *Handler) disable(w http.ResponseWriter, r *http.Request) {
	id, ok := h.project(w, r)
	if !ok {
		return
	}
	if err := h.store.Disable(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
