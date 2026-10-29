package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/hasangenc0/bordo/control-plane/internal/settings"
)

// githubClientID returns the OAuth App client id used for the GitHub device flow:
// BORDO_GITHUB_CLIENT_ID if set, else the github_client_id setting.
func (s *Server) githubClientID() string {
	if v := strings.TrimSpace(os.Getenv("BORDO_GITHUB_CLIENT_ID")); v != "" {
		return v
	}
	if s.settingsStore != nil {
		return strings.TrimSpace(s.settingsStore.GetOrDefault(settings.KeyGithubClientID, ""))
	}
	return ""
}

// handleConsoleSetupStatus handles GET /v1/setup/status — what the console needs to
// decide between the first-run screen, the GitHub prompt and the normal UI.
func (s *Server) handleConsoleSetupStatus(w http.ResponseWriter, r *http.Request) {
	configured := false
	if s.settingsStore != nil {
		configured, _ = s.settingsStore.IsConfigured()
	}
	connected := false
	if s.ghStore != nil {
		if tok, _, err := s.ghStore.LoadUserToken(r.Context()); err == nil && tok != "" {
			connected = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":          configured,
		"github_connected":    connected,
		"github_device_ready": s.githubClientID() != "",
	})
}

// handleGithubDeviceStart handles POST /v1/github/device/start.
func (s *Server) handleGithubDeviceStart(w http.ResponseWriter, r *http.Request) {
	// A JSON-only content type keeps cross-site form posts from starting a flow.
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return
	}
	if s.deviceFlow == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "github store unavailable"})
		return
	}
	cid := s.githubClientID()
	if cid == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{
			"error": "no GitHub OAuth App client id configured. Register an OAuth App with Device Flow enabled and set BORDO_GITHUB_CLIENT_ID, or paste a personal access token instead",
		})
		return
	}
	ds, err := s.deviceFlow.Start(cid)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

// handleGithubDeviceStatus handles GET /v1/github/device/status.
func (s *Server) handleGithubDeviceStatus(w http.ResponseWriter, r *http.Request) {
	if s.deviceFlow == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "idle"})
		return
	}
	writeJSON(w, http.StatusOK, s.deviceFlow.Status())
}
