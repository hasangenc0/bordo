package buildorchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// NewHandler returns a chi router for the /builds REST API.
// executor may be nil; when non-nil, triggered builds are executed asynchronously.
func NewHandler(store *Store, executor *Executor) http.Handler {
	r := chi.NewRouter()
	h := &handler{store: store, executor: executor}
	r.Post("/", h.trigger)
	r.Post("/enable", h.enable)
	r.Post("/repo", h.createRepo)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Get("/{id}/logs", h.logs)
	return r
}

type handler struct {
	store    *Store
	executor *Executor
}

type triggerRequest struct {
	ProjectID   string `json:"project_id"`
	ImageName   string `json:"image_name"`
	ImageTag    string `json:"image_tag"`
	RegistryURL string `json:"registry_url"`
}

func (h *handler) trigger(w http.ResponseWriter, r *http.Request) {
	var req triggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProjectID == "" {
		writeErr(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if req.ImageTag == "" {
		req.ImageTag = "latest"
	}
	// The project must exist. Accept its UUID or exact name; without this check a typo or a
	// deleted project produced a "successful" build with no image.
	var id, name string
	row := h.store.db.QueryRowContext(r.Context(), `SELECT id, name FROM projects WHERE id = ? OR name = ? LIMIT 1`, req.ProjectID, req.ProjectID)
	if err := row.Scan(&id, &name); err != nil {
		writeErr(w, http.StatusNotFound, "project not found (use the id or exact name from project_list)")
		return
	}
	req.ProjectID = id
	if req.ImageName == "" {
		req.ImageName = name
	}

	b, err := h.store.Create(r.Context(), req.ProjectID, req.ImageName, req.ImageTag)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Fire-and-forget: run the build pipeline asynchronously.
	if h.executor != nil {
		h.executor.Execute(b.ID)
	}

	writeJSON(w, http.StatusCreated, b)
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID != "" {
		// The agent passes names or truncated ids; map them to the project's id.
		var id string
		if err := h.store.db.QueryRowContext(r.Context(),
			`SELECT id FROM projects WHERE id = ? OR name = ? OR id LIKE ? ORDER BY (id = ?) DESC LIMIT 1`,
			projectID, projectID, projectID+"%", projectID).Scan(&id); err == nil {
			projectID = id
		}
	}
	builds, err := h.store.ListByProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	views := make([]buildView, 0, len(builds))
	for _, b := range builds {
		views = append(views, h.view(r.Context(), b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"builds": views})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	writeJSON(w, http.StatusOK, h.view(r.Context(), b))
}

func (h *handler) logs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	lines, err := h.store.GetLogs(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if lines == nil {
		lines = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

// buildView is a build plus a warning when it produced nothing new.
type buildView struct {
	*Build
	Warning string `json:"warning,omitempty"`
}

// view flags a successful build whose image is identical to an earlier build's. The image is named
// after the commit, so that means the default branch has not moved: a merge that "didn't build"
// usually never reached it, and calling the old image a fresh build misleads whoever deploys it.
func (h *handler) view(ctx context.Context, b *Build) buildView {
	v := buildView{Build: b}
	if b.Status != "success" || b.ImageDigest == "" {
		return v
	}
	var prev string
	var at time.Time
	err := h.store.db.QueryRowContext(ctx,
		`SELECT id, created_at FROM builds
		 WHERE project_id = ? AND status = 'success' AND image_digest = ? AND id != ? AND created_at < ?
		 ORDER BY created_at DESC LIMIT 1`,
		b.ProjectID, b.ImageDigest, b.ID, b.CreatedAt).Scan(&prev, &at)
	if err == nil {
		v.Warning = fmt.Sprintf("This is the same image as the earlier build %s (%s): the default branch has not moved since, "+
			"so none of the changes made after that commit are in it. If a pull request was merged, check that it actually "+
			"landed on the default branch (code_pr_status) before deploying this.", prev, at.UTC().Format(time.RFC3339))
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type enableRequest struct {
	ProjectID string `json:"project_id"`
}

// enable adds the Bordo build workflow to an imported project's GitHub repo.
func (h *handler) enable(w http.ResponseWriter, r *http.Request) {
	var req enableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProjectID == "" {
		writeErr(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if h.executor == nil {
		writeErr(w, http.StatusServiceUnavailable, "builds are not available")
		return
	}
	repo, path, err := h.executor.EnableBuilds(r.Context(), req.ProjectID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "repo": repo, "workflow": path})
}

// createRepo creates and links the GitHub repo for a template project.
func (h *handler) createRepo(w http.ResponseWriter, r *http.Request) {
	var req enableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProjectID == "" {
		writeErr(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if h.executor == nil {
		writeErr(w, http.StatusServiceUnavailable, "builds are not available")
		return
	}
	url, created, err := h.executor.CreateProjectRepo(r.Context(), req.ProjectID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repo": url, "created": created})
}
