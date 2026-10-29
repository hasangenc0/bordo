// Package setup implements the first-run setup wizard API.
package setup

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/hasangenc0/bordo/control-plane/internal/settings"
)

// Handler serves the unauthenticated setup endpoints.
type Handler struct {
	store      *settings.Store
	setupToken string
	tokenUsed  bool
	mu         sync.Mutex
	logger     *slog.Logger
}

// New creates a Handler with a freshly generated one-time setup token.
func New(store *settings.Store, logger *slog.Logger) (*Handler, error) {
	tok, err := generateToken()
	if err != nil {
		return nil, err
	}
	return &Handler{store: store, setupToken: tok, logger: logger}, nil
}

// SetupToken returns the one-time setup token (log it on startup).
func (h *Handler) SetupToken() string { return h.setupToken }

// StatusResponse is returned by GET /setup/status.
type StatusResponse struct {
	Configured bool `json:"configured"`
}

// SetupRequest is the POST /setup body.
type SetupRequest struct {
	SetupToken  string `json:"setup_token"`
	AdminToken  string `json:"admin_token,omitempty"` // auto-generated when empty
	DeepSeekKey string `json:"deepseek_api_key"`
	BaseURL     string `json:"base_url"`
	LLMModel    string `json:"llm_model,omitempty"`
	LLMURL      string `json:"llm_url,omitempty"`
	RegistryURL string `json:"registry_url,omitempty"`
}

// SetupResponse is returned by POST /setup on success.
type SetupResponse struct {
	AdminToken string `json:"admin_token"`
	Message    string `json:"message"`
}

// HandleStatus handles GET /setup/status.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	configured, err := h.store.IsConfigured()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, StatusResponse{Configured: configured})
}

// HandleSetup handles POST /setup — one-time first-run configuration.
func (h *Handler) HandleSetup(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.tokenUsed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "setup token already used"})
		return
	}

	configured, err := h.store.IsConfigured()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if configured {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already configured — use settings to update"})
		return
	}

	var req SetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.SetupToken != h.setupToken {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid setup token"})
		return
	}
	resp, status, msg := h.save(req)
	if msg != "" {
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleConsoleSetup handles POST /v1/setup — first-run configuration from the
// web console. It sits behind the bearer-token middleware (the console proxy
// adds the internal token), so no log-printed setup token is needed. It only
// works until setup has completed, and requires a JSON body so a cross-site
// form post cannot trigger it.
func (h *Handler) HandleConsoleSetup(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	configured, err := h.store.IsConfigured()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if configured {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already configured — use settings to update"})
		return
	}
	var req SetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	resp, status, msg := h.save(req)
	if msg != "" {
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	// The admin token is for the CLI; the console runs on the internal token and
	// must not hand a new credential to the browser.
	resp.AdminToken = ""
	resp.Message = "Setup complete."
	writeJSON(w, http.StatusOK, resp)
}

// save validates req, stores the settings and burns the setup token. The caller
// holds h.mu and has already checked authorization and the configured state.
// A non-empty message means failure, with the HTTP status to return.
func (h *Handler) save(req SetupRequest) (SetupResponse, int, string) {
	if req.DeepSeekKey == "" {
		return SetupResponse{}, http.StatusBadRequest, "deepseek_api_key is required"
	}

	adminToken := req.AdminToken
	if adminToken == "" {
		var err error
		adminToken, err = generateToken()
		if err != nil {
			return SetupResponse{}, http.StatusInternalServerError, "generating admin token"
		}
	}

	cfg := map[string]string{
		settings.KeyAdminToken:  adminToken,
		settings.KeyDeepSeekKey: req.DeepSeekKey,
		settings.KeyBaseURL:     req.BaseURL,
	}
	if req.LLMModel != "" {
		cfg[settings.KeyLLMModel] = req.LLMModel
	}
	if req.LLMURL != "" {
		cfg[settings.KeyLLMURL] = req.LLMURL
	}
	if req.RegistryURL != "" {
		cfg[settings.KeyRegistryURL] = req.RegistryURL
	}

	for k, v := range cfg {
		if err := h.store.Set(k, v); err != nil {
			h.logger.Error("setup: saving setting", "key", k, "error", err)
			return SetupResponse{}, http.StatusInternalServerError, "saving settings"
		}
	}

	h.tokenUsed = true
	h.logger.Info("bordo setup complete")

	return SetupResponse{
		AdminToken: adminToken,
		Message:    "Setup complete. Save your admin token — it will not be shown again.",
	}, http.StatusOK, ""
}

func isJSON(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return ct == "application/json" || strings.HasPrefix(ct, "application/json;")
}

// HandleUpdateSetting handles POST /v1/settings — update a single setting (requires auth).
func (h *Handler) HandleUpdateSetting(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key and value required"})
		return
	}

	// Only allow updating known, non-dangerous keys.
	allowed := map[string]bool{
		settings.KeyDeepSeekKey:    true,
		settings.KeyBaseURL:        true,
		settings.KeyLLMModel:       true,
		settings.KeyLLMURL:         true,
		settings.KeyRegistryURL:    true,
		settings.KeyGithubClientID: true,
	}
	if !allowed[req.Key] {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "key not updatable via this endpoint"})
		return
	}

	if err := h.store.Set(req.Key, req.Value); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
