package github

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handler serves GitHub App setup, OAuth, and status endpoints.
type Handler struct {
	store   *Store
	baseURL string
}

// NewHandler creates a Handler for all /github routes.
func NewHandler(store *Store, baseURL string) *Handler {
	return &Handler{store: store, baseURL: strings.TrimRight(baseURL, "/")}
}

// Routes returns the chi.Router for mounting under /github.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/app/setup", h.serveSetup)
	r.Get("/app/callback", h.handleCallback)
	r.Get("/app/install", h.handleInstall)
	r.Get("/app/installed", h.handleInstalled)
	r.Get("/app/auth", h.handleAuth)
	r.Get("/app/auth/callback", h.handleAuthCallback)
	r.Get("/status", h.handleStatus)
	r.Get("/token", h.handleToken)
	return r
}

// GetInstallationTokenFromStore generates a fresh installation token using stored credentials.
// Used by the server to expose /v1/github/token.
func (h *Handler) GetInstallationTokenFromStore(ctx context.Context) (string, string, error) {
	creds, err := h.store.LoadApp(ctx)
	if err != nil || creds == nil {
		return "", "", err
	}
	installID, targetType, _, err := h.store.LoadInstallationID(ctx)
	if err != nil {
		return "", "", err
	}
	if installID != 0 {
		appJWT, err := GenerateJWT(creds.AppID, creds.PrivateKeyPEM)
		if err != nil {
			return "", "", fmt.Errorf("generate JWT: %w", err)
		}
		tok, err := GetInstallationToken(appJWT, installID)
		if err != nil {
			return "", "", err
		}
		_ = targetType
		return tok, "installation", nil
	}
	userToken, expiresAt, err := h.store.LoadUserToken(ctx)
	if err != nil || userToken == "" {
		return "", "", err
	}
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		return "", "", fmt.Errorf("user token expired")
	}
	return userToken, "user", nil
}

const setupPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Bordo — Connect GitHub</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 600px; margin: 80px auto; padding: 0 24px; color: #1a1a1a; }
  h1 { font-size: 1.5rem; margin-bottom: 8px; }
  p  { color: #555; line-height: 1.6; }
  ol { color: #555; line-height: 2; }
  form { margin-top: 32px; }
  button {
    background: #1a1a1a; color: #fff; border: none; padding: 12px 28px;
    border-radius: 8px; font-size: 1rem; cursor: pointer;
  }
  button:hover { background: #333; }
  .note { font-size: 0.85rem; color: #888; margin-top: 16px; }
</style>
</head>
<body>
<h1>Connect Bordo to GitHub</h1>
<p>Clicking the button below will register a private GitHub App on your account.
   Bordo will use it to create repositories and push scaffolded code — no personal
   access token needed.</p>
<ol>
  <li>Click <strong>Create GitHub App</strong></li>
  <li>Review the requested permissions and click <em>Create GitHub App</em> on GitHub</li>
  <li>Install the app on your account or organization</li>
  <li>You're done — Bordo can now create repos on your behalf</li>
</ol>
<form action="https://github.com/settings/apps/new?state={{.State}}" method="post">
  <input type="hidden" name="manifest" value="{{.Manifest}}">
  <button type="submit">Create GitHub App →</button>
</form>
<p class="note">The app is private and will only be visible to your GitHub account.</p>
</body>
</html>`

func (h *Handler) serveSetup(w http.ResponseWriter, r *http.Request) {
	nonce := randomString(12)
	manifest := h.buildManifest(nonce)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tmpl := template.Must(template.New("setup").Parse(setupPageHTML))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(w, map[string]string{
		"State":    nonce,
		"Manifest": string(manifestJSON),
	})
}

func (h *Handler) buildManifest(nonce string) map[string]any {
	suffix := randomString(6)
	return map[string]any{
		"name": "Bordo-" + suffix,
		"url":  h.baseURL,
		"hook_attributes": map[string]any{
			"url":    h.baseURL + "/github/webhook",
			"active": false,
		},
		"redirect_url":     h.baseURL + "/github/app/callback",
		"callback_urls":    []string{h.baseURL + "/github/app/auth/callback"},
		"setup_url":        h.baseURL + "/github/app/installed",
		"setup_on_update":  true,
		"public":           false,
		"default_permissions": map[string]any{
			"contents":       "write",
			"metadata":       "read",
			"administration": "write",
		},
		"default_events": []string{},
	}
}

func (h *Handler) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	// Exchange manifest code for app credentials.
	apiURL := "https://api.github.com/app-manifests/" + code + "/conversions"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, apiURL, nil)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "GitHub API error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var result struct {
		ID            int64  `json:"id"`
		Slug          string `json:"slug"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		PEM           string `json:"pem"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		http.Error(w, "decode error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	creds := AppCredentials{
		AppID:         result.ID,
		AppSlug:       result.Slug,
		ClientID:      result.ClientID,
		ClientSecret:  result.ClientSecret,
		PrivateKeyPEM: result.PEM,
		WebhookSecret: result.WebhookSecret,
	}
	if err := h.store.SaveApp(r.Context(), creds); err != nil {
		http.Error(w, "store error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect to install page.
	http.Redirect(w, r, h.baseURL+"/github/app/install", http.StatusFound)
}

func (h *Handler) handleInstall(w http.ResponseWriter, r *http.Request) {
	creds, err := h.store.LoadApp(r.Context())
	if err != nil || creds == nil {
		http.Error(w, "GitHub App not registered — visit /github/app/setup first", http.StatusBadRequest)
		return
	}
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new", creds.AppSlug)
	http.Redirect(w, r, installURL, http.StatusFound)
}

func (h *Handler) handleInstalled(w http.ResponseWriter, r *http.Request) {
	installIDStr := r.URL.Query().Get("installation_id")
	if installIDStr == "" {
		http.Error(w, "missing installation_id", http.StatusBadRequest)
		return
	}
	installID, err := strconv.ParseInt(installIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid installation_id", http.StatusBadRequest)
		return
	}

	if err := h.store.SaveInstallationID(r.Context(), installID, "unknown", ""); err != nil {
		http.Error(w, "store error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Bordo — GitHub Connected</title>
<style>body{font-family:system-ui,sans-serif;max-width:500px;margin:80px auto;padding:0 24px;}
h1{color:#16a34a;}p{color:#555;line-height:1.6;}</style></head>
<body>
<h1>✓ GitHub App installed!</h1>
<p>Installation ID: <strong>%d</strong></p>
<p>Bordo can now create GitHub repositories on your behalf. You can close this tab.</p>
<p><a href="%s/github/status" style="color:#1a1a1a">Check status →</a></p>
</body></html>`, installID, h.baseURL)
}

func (h *Handler) handleAuth(w http.ResponseWriter, r *http.Request) {
	creds, err := h.store.LoadApp(r.Context())
	if err != nil || creds == nil {
		http.Error(w, "GitHub App not registered", http.StatusBadRequest)
		return
	}
	oauthURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=repo",
		creds.ClientID,
		h.baseURL+"/github/app/auth/callback",
	)
	http.Redirect(w, r, oauthURL, http.StatusFound)
}

func (h *Handler) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	creds, err := h.store.LoadApp(r.Context())
	if err != nil || creds == nil {
		http.Error(w, "GitHub App not registered", http.StatusBadRequest)
		return
	}
	token, err := ExchangeOAuthCode(creds.ClientID, creds.ClientSecret, code)
	if err != nil {
		http.Error(w, "OAuth error: "+err.Error(), http.StatusBadGateway)
		return
	}
	// GitHub user tokens don't expire by default unless token expiration is enabled on the app.
	if err := h.store.SaveUserToken(r.Context(), token, "", time.Time{}); err != nil {
		http.Error(w, "store error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Bordo — GitHub Authorized</title>
<style>body{font-family:system-ui,sans-serif;max-width:500px;margin:80px auto;padding:0 24px;}
h1{color:#16a34a;}p{color:#555;}</style></head>
<body>
<h1>✓ GitHub authorized!</h1>
<p>Your GitHub account is now connected to Bordo. You can close this tab.</p>
</body></html>`)
}

// ServeStatus is the public version used by the server's /v1/github/status route.
func (h *Handler) ServeStatus(w http.ResponseWriter, r *http.Request) {
	h.handleStatus(w, r)
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	creds, _ := h.store.LoadApp(r.Context())
	installID, _, _, _ := h.store.LoadInstallationID(r.Context())
	userToken, _, _ := h.store.LoadUserToken(r.Context())

	appRegistered := creds != nil && creds.AppID != 0
	var appID int64
	var appSlug string
	if creds != nil {
		appID = creds.AppID
		appSlug = creds.AppSlug
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_registered":  appRegistered,
		"app_id":          appID,
		"app_slug":        appSlug,
		"installation_id": installID,
		"user_authorized": userToken != "",
	})
}

func (h *Handler) handleToken(w http.ResponseWriter, r *http.Request) {
	tok, tokType, err := h.GetInstallationTokenFromStore(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tok == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "github not configured"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok, "type": tokType})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randomString(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[r.Intn(len(chars))]
	}
	return string(b)
}
