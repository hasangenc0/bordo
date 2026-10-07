// Package server implements the bordod HTTP API server.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/hasangenc0/bordo/control-plane/internal/approval"
	"github.com/hasangenc0/bordo/control-plane/internal/buildorchestrator"
	"github.com/hasangenc0/bordo/control-plane/internal/config"
	"github.com/hasangenc0/bordo/control-plane/internal/environment"
	"github.com/hasangenc0/bordo/control-plane/internal/fleet"
	githubpkg "github.com/hasangenc0/bordo/control-plane/internal/github"
	"github.com/hasangenc0/bordo/control-plane/internal/observe"
	"github.com/hasangenc0/bordo/control-plane/internal/registry"
	"github.com/hasangenc0/bordo/control-plane/internal/release"
	"github.com/hasangenc0/bordo/control-plane/internal/secrets"
	"github.com/hasangenc0/bordo/control-plane/internal/settings"
	"github.com/hasangenc0/bordo/control-plane/internal/setup"
	"github.com/hasangenc0/bordo/control-plane/internal/store"
	bordobtemplate "github.com/hasangenc0/bordo/control-plane/internal/template"
	"github.com/hasangenc0/bordo/control-plane/internal/version"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server is the bordod HTTP API server.
type Server struct {
	cfg               *config.Config
	logger            *slog.Logger
	router            *chi.Mux
	db                *sql.DB
	http              *http.Server
	secretKey         [32]byte
	releaseController *release.Controller
	githubHandler     *githubpkg.Handler
	ghStore           *githubpkg.Store
	settingsStore     *settings.Store
	setupHandler      *setup.Handler
}

// New creates a Server wired with all routes.
func New(cfg *config.Config, logger *slog.Logger, db *sql.DB) *Server {
	secretKey, err := secrets.DeriveKey()
	if err != nil {
		logger.Warn("secrets: could not derive encryption key, secrets disabled", "error", err)
	}

	settingsStore, err := settings.New(db, cfg.Store.Path)
	if err != nil {
		logger.Warn("settings store unavailable", "error", err)
	}

	// Overlay settings-store values onto cfg so the rest of the server
	// can read them without knowing where they came from.
	if settingsStore != nil {
		if v := settingsStore.GetOrDefault(settings.KeyAdminToken, ""); v != "" && cfg.Server.AuthToken == "" {
			cfg.Server.AuthToken = v
		}
		if v := settingsStore.GetOrDefault(settings.KeyBaseURL, ""); v != "" && cfg.Server.BaseURL == "" {
			cfg.Server.BaseURL = v
		}
	}

	setupHdlr, err := setup.New(settingsStore, logger)
	if err != nil {
		logger.Error("setup handler unavailable", "error", err)
	}

	// On first run, print the one-time setup token so the admin can complete setup.
	if settingsStore != nil && setupHdlr != nil {
		if configured, _ := settingsStore.IsConfigured(); !configured {
			logger.Warn("BORDO NOT CONFIGURED — complete setup to start using Bordo",
				"setup_url", cfg.Server.GetBaseURL()+"/setup/status",
				"setup_token", setupHdlr.SetupToken(),
				"cli_hint", "bordo setup --server "+cfg.Server.GetBaseURL(),
			)
		}
	}

	ghStore := githubpkg.NewStore(db, secretKey)
	s := &Server{
		cfg:               cfg,
		logger:            logger,
		db:                db,
		router:            chi.NewRouter(),
		secretKey:         secretKey,
		releaseController: release.NewController(db, logger, ghStore),
		githubHandler:     githubpkg.NewHandler(ghStore, cfg.Server.GetBaseURL()),
		ghStore:           ghStore,
		settingsStore:     settingsStore,
		setupHandler:      setupHdlr,
	}
	s.registerMiddleware()
	s.registerRoutes()

	s.http = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// Start listens and serves. It returns when ctx is cancelled, after draining
// in-flight requests with a 15-second shutdown timeout.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.http.Addr, err)
	}

	s.logger.Info("bordod listening", "addr", s.http.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("bordod shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	}
}

// Addr returns the address the server is listening on (useful for tests).
func (s *Server) Addr() string {
	return s.http.Addr
}

// StartController starts the release controller in a goroutine.
func (s *Server) StartController(ctx context.Context) {
	go func() {
		if err := s.releaseController.Start(ctx); err != nil {
			s.logger.Error("release controller stopped", "err", err)
		}
	}()
}

// registerMiddleware adds global middleware to the router.
func (s *Server) registerMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(s.slogMiddleware())
	s.router.Use(middleware.Recoverer)
}

// requireToken returns a middleware that enforces bearer-token auth.
// The token is re-read from the settings store on every request so setup
// does not require a server restart.
func (s *Server) requireToken() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			admin := s.activeAdminToken()
			// Internal service token (from BORDO_AUTH_TOKEN env) — used by the
			// agent for service-to-service calls like /v1/runtime-config. It
			// stays valid even after setup stores a separate admin token.
			internal := s.cfg.Server.AuthToken
			if admin == "" && internal == "" {
				next.ServeHTTP(w, r)
				return
			}
			auth := r.Header.Get("Authorization")
			if (admin != "" && auth == "Bearer "+admin) || (internal != "" && auth == "Bearer "+internal) {
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		})
	}
}

// activeAdminToken returns the current admin token, preferring the settings
// store over the static config value (set via env var).
func (s *Server) activeAdminToken() string {
	if s.settingsStore != nil {
		if v := s.settingsStore.GetOrDefault(settings.KeyAdminToken, ""); v != "" {
			return v
		}
	}
	return s.cfg.Server.AuthToken
}

// registerRoutes wires all API routes.
func (s *Server) registerRoutes() {
	// Health / readiness probes — always public (used by load balancers).
	s.router.Get("/healthz", s.handleHealthz)
	s.router.Get("/readyz", s.handleReadyz)
	s.router.Get("/version", s.handleVersion)

	// Setup wizard — public, no auth required.
	// POST /setup is a one-time endpoint protected only by the setup token.
	if s.setupHandler != nil {
		s.router.Get("/setup/status", s.setupHandler.HandleStatus)
		s.router.Post("/setup", s.setupHandler.HandleSetup)
	}

	// GitHub App OAuth callbacks — must be reachable by the admin's browser
	// after GitHub redirects; no token needed (the user is on VPN).
	s.router.Mount("/github", s.githubHandler.Routes())

	// Versioned API — protected by bearer token when BORDO_AUTH_TOKEN is set.
	s.router.Route("/v1", func(r chi.Router) {
		r.Use(s.requireToken())
		r.Mount("/projects", registry.NewHandler(registry.New(s.db)))
		r.Mount("/regions", fleet.NewHandler(fleet.New(s.db), s.logger))
		buildStore := buildorchestrator.New(s.db)
		buildExecutor := buildorchestrator.NewExecutor(buildStore, s.db, s.logger, "", "", "", s.ghStore)
		r.Mount("/builds", buildorchestrator.NewHandler(buildStore, buildExecutor))
		r.Mount("/releases", release.NewHandler(s.db))

		secretsHandler := secrets.NewHandler(secrets.NewSQLiteStore(s.db, s.secretKey))
		r.Route("/projects/{projectID}/secrets", func(r chi.Router) {
			r.Post("/", secretsHandler.Set)
			r.Get("/", secretsHandler.List)
			r.Delete("/{key}", secretsHandler.Delete)
		})

		envStore := environment.NewStore(s.db)
		r.Mount("/projects/{projectID}/environments", environment.NewHandler(envStore))

		approvalStore := approval.NewStore(s.db)
		r.Mount("/approvals", approval.NewHandler(approvalStore))

		observeHandler := observe.NewHandler(s.db)
		r.Get("/observe/metrics", observeHandler.QueryMetrics)
		r.Get("/observe/logs", observeHandler.QueryLogs)
		r.Get("/observe/traces/{traceID}", observeHandler.QueryTrace)

		templateHandler := bordobtemplate.NewHandler(os.Getenv("BORDO_TEMPLATE_ROOT"))
		r.Mount("/templates", templateHandler)
		r.Get("/fleet", s.handleFleetOverview)

		// GitHub App integration helpers.
		r.Get("/github/token", s.handleGithubToken)
		r.Get("/github/status", s.handleGithubStatus)
		r.Post("/github/user-token", s.handleSetGithubUserToken)

		// Platform settings — update a single key (admin only).
		if s.setupHandler != nil {
			r.Post("/settings", s.setupHandler.HandleUpdateSetting)
		}

		// Runtime config endpoint for the agent — returns LLM provider config.
		r.Get("/runtime-config", s.handleRuntimeConfig)
	})
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := store.Ping(s.db); err != nil {
		s.logger.Warn("readyz: db ping failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version":   version.Version,
		"commit":    version.Commit,
		"buildTime": version.BuildTime,
	})
}

type templateInfo struct {
	Name        string `json:"name"`
	Runtime     string `json:"runtime"`
	Description string `json:"description"`
}

var builtinTemplates = []templateInfo{
	{"java-web-service", "java", "Spring Boot 3 REST service with OpenTelemetry and health actuator"},
	{"java-worker", "java", "Spring Boot background worker using the bordo-worker SDK"},
	{"java-kafka", "java", "Spring Boot Kafka consumer/producer using the bordo-kafka SDK"},
	{"java-job", "java", "Spring Boot scheduled/one-shot job with distributed lock via bordo-job SDK"},
	{"java-data-layer", "java", "HikariCP + Flyway data-access layer using the bordo-data SDK"},
	{"ts-react-app", "typescript", "Vite + React + TypeScript frontend with OpenTelemetry browser instrumentation"},
	{"node-bff", "node", "Fastify + TypeScript BFF (backend-for-frontend) with pino logging and OpenTelemetry"},
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": builtinTemplates})
}

func (s *Server) handleGithubToken(w http.ResponseWriter, r *http.Request) {
	tok, tokType, err := s.githubHandler.GetInstallationTokenFromStore(r.Context())
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

// handleSetGithubUserToken stores a GitHub user access token obtained by the
// CLI via the device flow (or a PAT). This is the self-hosted path: the token
// is the user's own, scoped to what they granted, and lets Bordo create repos,
// run builds in GitHub Actions, and pull private images.
func (s *Server) handleSetGithubUserToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}
	if s.ghStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "github store unavailable"})
		return
	}
	login, _ := githubpkg.GetCurrentUserLogin(req.Token)
	if err := s.ghStore.SaveUserToken(r.Context(), req.Token, login, time.Time{}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "connected", "login": login})
}

func (s *Server) handleGithubStatus(w http.ResponseWriter, r *http.Request) {
	// Delegate to the github handler's status logic by forwarding internally.
	s.githubHandler.ServeStatus(w, r)
}

func (s *Server) handleRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	type runtimeConfig struct {
		DeepSeekAPIKey string `json:"deepseek_api_key,omitempty"`
		LLMModel       string `json:"llm_model,omitempty"`
		LLMURL         string `json:"llm_url,omitempty"`
		RegistryURL    string `json:"registry_url,omitempty"`
		BaseURL        string `json:"base_url,omitempty"`
	}
	cfg := runtimeConfig{
		LLMModel: "deepseek-chat",
	}
	if s.settingsStore != nil {
		cfg.DeepSeekAPIKey = s.settingsStore.GetOrDefault(settings.KeyDeepSeekKey, "")
		cfg.LLMModel = s.settingsStore.GetOrDefault(settings.KeyLLMModel, cfg.LLMModel)
		cfg.LLMURL = s.settingsStore.GetOrDefault(settings.KeyLLMURL, "")
		cfg.RegistryURL = s.settingsStore.GetOrDefault(settings.KeyRegistryURL, "")
		cfg.BaseURL = s.settingsStore.GetOrDefault(settings.KeyBaseURL, "")
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleFleetOverview(w http.ResponseWriter, r *http.Request) {
	type deployment struct {
		ProjectID string `json:"project_id"`
		ImageTag  string `json:"image_tag"`
		Status    string `json:"status"`
		Region    string `json:"region"`
	}
	type regionOverview struct {
		Name        string       `json:"name"`
		Status      string       `json:"status"`
		Deployments []deployment `json:"deployments"`
	}

	rows, err := s.db.QueryContext(r.Context(), `SELECT name, status FROM regions ORDER BY name`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var regions []regionOverview
	for rows.Next() {
		var ro regionOverview
		if err := rows.Scan(&ro.Name, &ro.Status); err != nil {
			continue
		}
		ro.Deployments = []deployment{}
		regions = append(regions, ro)
	}
	rows.Close()

	// Fetch latest release per project per region.
	relRows, err := s.db.QueryContext(r.Context(),
		`SELECT project_id, image_tag, status, region FROM bordo_releases
		 WHERE id IN (
		   SELECT id FROM bordo_releases r2
		   WHERE r2.region = bordo_releases.region AND r2.project_id = bordo_releases.project_id
		   ORDER BY created_at DESC LIMIT 1
		 )`)
	if err == nil {
		defer relRows.Close()
		deploysByRegion := map[string][]deployment{}
		for relRows.Next() {
			var d deployment
			if err := relRows.Scan(&d.ProjectID, &d.ImageTag, &d.Status, &d.Region); err == nil {
				deploysByRegion[d.Region] = append(deploysByRegion[d.Region], d)
			}
		}
		for i := range regions {
			if deps, ok := deploysByRegion[regions[i].Name]; ok {
				regions[i].Deployments = deps
			}
		}
	}

	if regions == nil {
		regions = []regionOverview{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": regions})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Nothing we can do; response already started.
		_ = err
	}
}

// slogMiddleware returns a chi middleware that logs each request via slog.
func (s *Server) slogMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			defer func() {
				s.logger.Info("request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"duration", time.Since(start).String(),
					"request_id", middleware.GetReqID(r.Context()),
				)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}
