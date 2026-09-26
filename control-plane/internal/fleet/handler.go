package fleet

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type addRegionRequest struct {
	Name       string `json:"name"`
	Kubeconfig string `json:"kubeconfig"`
}

// NewHandler returns a chi router for the /regions REST API.
func NewHandler(store *Store) http.Handler {
	r := chi.NewRouter()
	h := &handler{store: store}
	r.Post("/", h.add)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Delete("/{id}", h.remove)
	return r
}

type handler struct {
	store *Store
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
