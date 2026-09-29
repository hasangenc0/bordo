package approval

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// NewHandler returns a chi router for approval endpoints.
// It is intended to be mounted at /v1/approvals.
func NewHandler(store ApprovalStore) http.Handler {
	r := chi.NewRouter()
	h := &approvalHandler{store: store}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Post("/{id}/approve", h.approve)
	r.Post("/{id}/reject", h.reject)
	return r
}

type approvalHandler struct {
	store ApprovalStore
}

func (h *approvalHandler) list(w http.ResponseWriter, r *http.Request) {
	approvals, err := h.store.ListPendingApprovals(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if approvals == nil {
		approvals = []*Approval{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": approvals})
}

type createApprovalRequest struct {
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Payload string `json:"payload"`
}

func (h *approvalHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Kind = strings.TrimSpace(req.Kind)
	req.Target = strings.TrimSpace(req.Target)
	if req.Kind == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind is required"})
		return
	}
	if req.Target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target is required"})
		return
	}

	a := &Approval{
		Kind:    req.Kind,
		Target:  req.Target,
		Payload: req.Payload,
	}
	if err := h.store.CreateApproval(r.Context(), a); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (h *approvalHandler) approve(w http.ResponseWriter, r *http.Request) {
	h.resolve(w, r, "approved")
}

func (h *approvalHandler) reject(w http.ResponseWriter, r *http.Request) {
	h.resolve(w, r, "rejected")
}

func (h *approvalHandler) resolve(w http.ResponseWriter, r *http.Request, status string) {
	id := chi.URLParam(r, "id")
	if err := h.store.ResolveApproval(r.Context(), id, status); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	a, err := h.store.GetApproval(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": status})
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
