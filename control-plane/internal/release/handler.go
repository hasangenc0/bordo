// Package release implements the release/deploy HTTP handlers for the control plane.
package release

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Release is a deployment record.
type Release struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	ImageTag  string    `json:"image_tag"`
	Region    string    `json:"region"`
	Strategy  string    `json:"strategy"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Handler exposes release CRUD over HTTP.
type Handler struct {
	db *sql.DB
}

// New creates a Handler.
func New(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// NewHandler returns a chi.Router with release routes mounted.
func NewHandler(db *sql.DB) http.Handler {
	h := New(db)
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Post("/{id}/rollback", h.Rollback)
	return r
}

type createRequest struct {
	ProjectID string `json:"project_id"`
	ImageTag  string `json:"image_tag"`
	Region    string `json:"region"`
	Strategy  string `json:"strategy"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.ProjectID == "" || req.ImageTag == "" || req.Region == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id, image_tag, and region are required"})
		return
	}
	if req.Strategy == "" {
		req.Strategy = "rolling"
	}

	rel := &Release{
		ID:        uuid.New().String(),
		ProjectID: req.ProjectID,
		ImageTag:  req.ImageTag,
		Region:    req.Region,
		Strategy:  req.Strategy,
		Status:    "reconciling",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	const q = `INSERT INTO bordo_releases (id, project_id, image_tag, region, strategy, status, created_at, updated_at)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := h.db.ExecContext(r.Context(), q,
		rel.ID, rel.ProjectID, rel.ImageTag, rel.Region, rel.Strategy, rel.Status,
		rel.CreatedAt.Format(time.RFC3339), rel.UpdatedAt.Format(time.RFC3339),
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, rel)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var rel Release
	var createdAt, updatedAt string
	const q = `SELECT id, project_id, image_tag, region, strategy, status, created_at, updated_at
               FROM bordo_releases WHERE id = ?`
	row := h.db.QueryRowContext(r.Context(), q, id)
	if err := row.Scan(&rel.ID, &rel.ProjectID, &rel.ImageTag, &rel.Region, &rel.Strategy, &rel.Status,
		&createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "release not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rel.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	rel.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `UPDATE bordo_releases SET status = 'rolling_back', updated_at = ? WHERE id = ?`
	result, err := h.db.ExecContext(r.Context(), q, now, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "release not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "rolling_back"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
