package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	githubpkg "github.com/hasangenc0/bordo/control-plane/internal/github"
)

type createRequest struct {
	Name       string `json:"name"`
	Template   string `json:"template"`
	GitRepoURL string `json:"git_repo_url"` // import an existing GitHub repo instead of a template
}

// NewHandler returns a chi router implementing the /projects REST API.
func NewHandler(store Store) http.Handler {
	r := chi.NewRouter()
	h := &handler{store: store}
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Delete("/{id}", h.delete)
	r.Get("/{id}/env", h.getEnv)
	r.Put("/{id}/env", h.putEnv)
	return r
}

type handler struct {
	store Store
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	repoURL := ""
	if strings.TrimSpace(req.GitRepoURL) != "" {
		// An imported repo and a template are mutually exclusive: builds scaffold the
		// template into the repo and force-push, which would overwrite imported code.
		if strings.TrimSpace(req.Template) != "" {
			writeErr(w, http.StatusBadRequest, "choose either a template or an existing repository, not both")
			return
		}
		canonical, _, repoName, err := githubpkg.NormalizeRepoURL(req.GitRepoURL)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		repoURL = canonical
		if req.Name == "" {
			req.Name = repoName
		}
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	p := &Project{
		Name:       req.Name,
		Template:   req.Template,
		GitRepoURL: repoURL,
	}
	if err := h.store.CreateProject(r.Context(), p); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/v1/projects/%s", p.ID))
	writeJSON(w, http.StatusCreated, p)
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	projects, err := h.store.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if projects == nil {
		projects = []*Project{}
	}

	if r.URL.Query().Get("include") == "release_status" {
		enriched := make([]*ProjectWithStatus, 0, len(projects))
		for _, p := range projects {
			pws := &ProjectWithStatus{Project: p}
			status, region, _, _ := h.store.GetLatestReleaseInfo(r.Context(), p.ID)
			pws.ReleaseStatus = status
			pws.ReleaseRegion = region
			enriched = append(enriched, pws)
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": enriched})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.store.GetProject(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	pws := &ProjectWithStatus{Project: p}
	status, region, _, _ := h.store.GetLatestReleaseInfo(r.Context(), id)
	pws.ReleaseStatus = status
	pws.ReleaseRegion = region
	writeJSON(w, http.StatusOK, pws)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.store.DeleteProject(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) getEnv(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Ensure project exists.
	if _, err := h.store.GetProject(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, http.StatusNotFound, "project not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	vars, err := h.store.GetEnvVars(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if vars == nil {
		vars = []*EnvVar{}
	}

	// Mask secret values.
	for _, v := range vars {
		if v.Secret {
			v.Value = "***"
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"vars": vars})
}

type putEnvRequest struct {
	Vars []*EnvVar `json:"vars"`
}

func (h *handler) putEnv(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Ensure project exists.
	if _, err := h.store.GetProject(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, http.StatusNotFound, "project not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req putEnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Vars == nil {
		req.Vars = []*EnvVar{}
	}

	if err := h.store.SetEnvVars(r.Context(), id, req.Vars); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"vars": req.Vars})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
