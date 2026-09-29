package template

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

// Handler implements the /v1/templates endpoint.
type Handler struct {
	root string
}

// NewHandler creates a chi Handler that reads templates from root directory.
// If root is empty, it falls back to BORDO_TEMPLATE_ROOT env var, then "./templates".
func NewHandler(root string) http.Handler {
	if root == "" {
		root = os.Getenv("BORDO_TEMPLATE_ROOT")
	}
	if root == "" {
		root = "./templates"
	}
	h := &Handler{root: root}
	r := chi.NewRouter()
	r.Get("/", h.list)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.root)
	if err != nil {
		// If root doesn't exist, return empty list rather than an error.
		writeJSON(w, http.StatusOK, map[string]any{"templates": []*TemplateMeta{}})
		return
	}

	var templates []*TemplateMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(h.root, e.Name(), "template.yaml")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			// Skip dirs without template.yaml.
			continue
		}
		meta, err := parseMeta(data)
		if err != nil {
			// Skip malformed metadata.
			continue
		}
		if meta.Variables == nil {
			meta.Variables = []TemplateVar{}
		}
		templates = append(templates, meta)
	}

	if templates == nil {
		templates = []*TemplateMeta{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
