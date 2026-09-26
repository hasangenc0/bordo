package buildorchestrator

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewHandler returns a chi router for the /builds REST API.
// executor may be nil; when non-nil, triggered builds are executed asynchronously.
func NewHandler(store *Store, executor *Executor) http.Handler {
	r := chi.NewRouter()
	h := &handler{store: store, executor: executor}
	r.Post("/", h.trigger)
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
	builds, err := h.store.ListByProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if builds == nil {
		builds = []*Build{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"builds": builds})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	writeJSON(w, http.StatusOK, b)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
