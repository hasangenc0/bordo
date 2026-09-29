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
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	ImageTag       string    `json:"image_tag"`
	Region         string    `json:"region"`
	Strategy       string    `json:"strategy"`
	Status         string    `json:"status"`
	ReleaseGroupID string    `json:"release_group_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/group/{groupID}", h.GetGroup)
	r.Get("/{id}", h.Get)
	r.Get("/{id}/logs", h.GetLogs)
	r.Post("/{id}/rollback", h.Rollback)
	return r
}

type createRequest struct {
	ProjectID string   `json:"project_id"`
	ImageTag  string   `json:"image_tag"`
	Region    string   `json:"region"`
	Regions   []string `json:"regions"`
	Strategy  string   `json:"strategy"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	var (
		rows *sql.Rows
		err  error
	)
	const qAll = `SELECT id, project_id, image_tag, region, strategy, status, release_group_id, created_at, updated_at
	               FROM bordo_releases ORDER BY created_at DESC LIMIT 50`
	const qByProject = `SELECT id, project_id, image_tag, region, strategy, status, release_group_id, created_at, updated_at
	               FROM bordo_releases WHERE project_id = ? ORDER BY created_at DESC LIMIT 50`
	if projectID != "" {
		rows, err = h.db.QueryContext(r.Context(), qByProject, projectID)
	} else {
		rows, err = h.db.QueryContext(r.Context(), qAll)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	releases := make([]Release, 0)
	for rows.Next() {
		rel, err := scanRelease(rows)
		if err == nil {
			releases = append(releases, rel)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": releases})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.ProjectID == "" || req.ImageTag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and image_tag are required"})
		return
	}
	if req.Strategy == "" {
		req.Strategy = "rolling"
	}

	// Collect target regions.
	regions := req.Regions
	if len(regions) == 0 && req.Region != "" {
		regions = []string{req.Region}
	}
	if len(regions) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "region or regions is required"})
		return
	}

	groupID := uuid.New().String()
	created := make([]Release, 0, len(regions))
	now := time.Now().UTC()

	for _, reg := range regions {
		rel := Release{
			ID:             uuid.New().String(),
			ProjectID:      req.ProjectID,
			ImageTag:       req.ImageTag,
			Region:         reg,
			Strategy:       req.Strategy,
			Status:         "reconciling",
			ReleaseGroupID: groupID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		const q = `INSERT INTO bordo_releases (id, project_id, image_tag, region, strategy, status, release_group_id, created_at, updated_at)
		           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		if _, err := h.db.ExecContext(r.Context(), q,
			rel.ID, rel.ProjectID, rel.ImageTag, rel.Region, rel.Strategy, rel.Status,
			rel.ReleaseGroupID,
			rel.CreatedAt.Format(time.RFC3339), rel.UpdatedAt.Format(time.RFC3339),
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		created = append(created, rel)
	}

	if len(created) == 1 {
		writeJSON(w, http.StatusCreated, created[0])
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"release_group_id": groupID,
		"releases":         created,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row := h.db.QueryRowContext(r.Context(),
		`SELECT id, project_id, image_tag, region, strategy, status, release_group_id, created_at, updated_at
		 FROM bordo_releases WHERE id = ?`, id)
	rel, err := scanReleaseRow(row)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "release not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) GetLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var log string
	err := h.db.QueryRowContext(r.Context(), `SELECT log FROM bordo_releases WHERE id = ?`, id).Scan(&log)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "release not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "log": log})
}

func (h *Handler) GetGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, project_id, image_tag, region, strategy, status, release_group_id, created_at, updated_at
		 FROM bordo_releases WHERE release_group_id = ? ORDER BY created_at DESC`, groupID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	releases := make([]Release, 0)
	for rows.Next() {
		rel, err := scanRelease(rows)
		if err == nil {
			releases = append(releases, rel)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"release_group_id": groupID, "releases": releases})
}

func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := h.db.ExecContext(r.Context(),
		`UPDATE bordo_releases SET status = 'rolling_back', updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "release not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "rolling_back"})
}

// scanRelease scans a *sql.Rows into a Release.
func scanRelease(rows *sql.Rows) (Release, error) {
	var rel Release
	var createdAt, updatedAt string
	if err := rows.Scan(&rel.ID, &rel.ProjectID, &rel.ImageTag, &rel.Region, &rel.Strategy, &rel.Status,
		&rel.ReleaseGroupID, &createdAt, &updatedAt); err != nil {
		return Release{}, err
	}
	rel.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	rel.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return rel, nil
}

// scanReleaseRow scans a *sql.Row into a Release.
func scanReleaseRow(row *sql.Row) (Release, error) {
	var rel Release
	var createdAt, updatedAt string
	if err := row.Scan(&rel.ID, &rel.ProjectID, &rel.ImageTag, &rel.Region, &rel.Strategy, &rel.Status,
		&rel.ReleaseGroupID, &createdAt, &updatedAt); err != nil {
		return Release{}, err
	}
	rel.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	rel.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return rel, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
