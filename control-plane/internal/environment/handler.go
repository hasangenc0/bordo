package environment

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// NewHandler returns a chi router for environment endpoints.
// It is intended to be mounted at /v1/projects/{projectID}/environments.
func NewHandler(store EnvironmentStore) http.Handler {
	r := chi.NewRouter()
	h := &envHandler{store: store}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Delete("/{envID}", h.delete)
	return r
}

type envHandler struct {
	store EnvironmentStore
}

func (h *envHandler) list(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	envs, err := h.store.ListEnvironments(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if envs == nil {
		envs = []*Environment{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"environments": envs})
}

type createEnvRequest struct {
	Name       string `json:"name"`
	Branch     string `json:"branch"`
	RegionName string `json:"region_name"`
}

func (h *envHandler) create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req createEnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	env := &Environment{
		ProjectID:  projectID,
		Name:       req.Name,
		Branch:     req.Branch,
		RegionName: req.RegionName,
	}
	if err := h.store.CreateEnvironment(r.Context(), env); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, env)
}

func (h *envHandler) delete(w http.ResponseWriter, r *http.Request) {
	envID := chi.URLParam(r, "envID")
	if err := h.store.DeleteEnvironment(r.Context(), envID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "environment not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
