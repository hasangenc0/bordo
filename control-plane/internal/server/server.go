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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

	// workloads holds the latest runtime snapshot each region's reconciler
	// reported (services, nodePorts, pod states). In memory only: the reconciler
	// re-reports every cycle.
	workloadsMu sync.RWMutex
	workloads   map[string]workloadReport
	// pullNotes records, per region, whether the GHCR pull secret could be shipped on the last
	// desired-state pull, so a missing or expired GitHub login shows up next to the pod's
	// ImagePullBackOff instead of staying a guess.
	pullNotes map[string]string
}

// workloadReport is a region's reconciler-reported runtime snapshot.
type workloadReport struct {
	Region     string    `json:"region"`
	ReportedAt time.Time `json:"reported_at"`
	// HostAddr is the address the reconciler's requests came from, i.e. the region
	// machine as the control plane sees it (a NAT or tunnel address if one is in the way).
	HostAddr string        `json:"host_addr,omitempty"`
	URLs     []string      `json:"urls,omitempty"`
	Services []observedSvc `json:"services"`
	Pods     []observedPod `json:"pods"`
	// PullSecret says whether the private-image pull secret was shipped to this region.
	PullSecret string `json:"pull_secret,omitempty"`
}

type observedSvc struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Port      int    `json:"port"`
	NodePorts []int  `json:"node_ports,omitempty"`
}

type observedPod struct {
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	Ready   bool   `json:"ready"`
	Reason  string `json:"reason,omitempty"`
	Image   string `json:"image,omitempty"`
	Message string `json:"message,omitempty"`
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

	// Node-join redeem (ADR-0006 Phase 1) — node-facing, no admin token. It is
	// gated by the single-use join token in the body; the node agent calls it
	// outbound during bootstrap.
	s.router.Post("/fleet/redeem", s.handleFleetRedeem)
	s.router.Post("/fleet/register", s.handleFleetRegister)
	s.router.Get("/fleet/desired-state", s.handleFleetDesiredState)
	// The on-cluster reconciler reports back which releases it applied, closing
	// the pull-model feedback loop (ADR-0006 Phase 2). Cluster-token gated.
	s.router.Post("/fleet/status", s.handleFleetStatus)

	// Versioned API — protected by bearer token when BORDO_AUTH_TOKEN is set.
	s.router.Route("/v1", func(r chi.Router) {
		r.Use(s.requireToken())
		r.Mount("/projects", registry.NewHandler(registry.New(s.db)))
		r.Mount("/regions", fleet.NewHandler(fleet.New(s.db), s.logger))
		buildStore := buildorchestrator.New(s.db)
		buildExecutor := buildorchestrator.NewExecutor(buildStore, s.db, s.logger, "", "", "", s.ghStore)
		go buildExecutor.ResumeInterrupted(context.Background())
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
		r.Get("/fleet/workloads", s.handleFleetWorkloads)

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
	{"go-web-service", "go", "Go HTTP service on stdlib net/http; static binary, low memory, fast cold start"},
	{"go-job", "go", "Go scheduled job runner with graceful shutdown; static binary, low memory"},
	{"go-kafka", "go", "Go Kafka consumer/producer on segmentio/kafka-go; static binary, low memory"},
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
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		ClientID     string `json:"client_id"`
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
	// An expiring token is stored with its refresh token so the login renews itself.
	if err := s.ghStore.SaveUserSession(r.Context(), githubpkg.UserSession{
		Token: req.Token, RefreshToken: req.RefreshToken, ClientID: req.ClientID, ExpiresAt: githubpkg.ExpiryFrom(req.ExpiresIn),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]string{"status": "connected", "login": login}
	if problem := githubpkg.PullTokenProblem(req.Token); problem != "" {
		resp["warning"] = "private image pulls will fail: " + problem
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleFleetRedeem consumes a single-use node-join token (ADR-0006 Phase 1).
// The response is uniform on any failure so it cannot be used as an oracle to
// distinguish "wrong token" from "expired"/"used"/"revoked".
func (s *Server) handleFleetRedeem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}
	st := fleet.New(s.db)
	region, err := st.RedeemJoinToken(r.Context(), req.Token)
	if err != nil {
		s.logger.Warn("fleet join redeem rejected", "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired join token"})
		return
	}
	// Issue a single-use register token (same tested mechanism) that the node
	// uses to complete registration after k3s is up. Only the successful
	// redeemer receives it, so register is gated to that one node.
	_, registerToken, err := st.MintJoinToken(r.Context(), region, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"region":         region,
		"register_token": registerToken,
		"status":         "redeemed",
	})
}

// handleFleetRegister completes node registration: the node sends its cluster
// kubeconfig along with the single-use register token it got from redeem. The
// token both authenticates the call and pins it to one region.
//
// NOTE: storing the kubeconfig here is transitional — ADR-0006 Phase 2 replaces
// it with an on-cluster pull reconciler so the control plane holds no admin
// kubeconfig at all.
func (s *Server) handleFleetRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Region        string `json:"region"`
		Kubeconfig    string `json:"kubeconfig"`
		RegisterToken string `json:"register_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Region == "" || req.Kubeconfig == "" || req.RegisterToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "region, kubeconfig, register_token required"})
		return
	}
	st := fleet.New(s.db)
	region, err := st.RedeemJoinToken(r.Context(), req.RegisterToken)
	if err != nil || region != req.Region {
		s.logger.Warn("fleet register rejected", "error", err, "region", req.Region)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired register token"})
		return
	}
	// Mark the region 'pending', not 'healthy': the node has joined but hasn't
	// proven it's live yet. The first successful desired-state pull from its
	// on-cluster reconciler flips it to 'healthy' (recordRegionPull); if pulls
	// then stop, health derivation downgrades it to 'unreachable'. This is what
	// keeps a dead region from reading healthy forever.
	if err := st.UpdateKubeconfig(r.Context(), req.Region, req.Kubeconfig, "pending"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Issue the region's reconciler credential (ADR-0006 Phase 2): a revocable,
	// region-scoped token the on-cluster reconciler uses to pull desired state.
	clusterToken, err := st.IssueClusterToken(r.Context(), req.Region)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":        "registered",
		"region":        region,
		"cluster_token": clusterToken,
	})
}

// handleFleetDesiredState serves the manifests a region should be running
// (ADR-0006 Phase 2). It is pulled by the on-cluster reconciler and gated by the
// region's cluster token (uniform 401 on any failure).
func (s *Server) handleFleetDesiredState(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "cluster token required"})
		return
	}
	region, err := fleet.New(s.db).ValidateClusterToken(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid cluster token"})
		return
	}
	// A successful authenticated pull is the region's liveness signal; record it
	// so health is derived from reality, not latched at join time.
	s.recordRegionPull(r.Context(), region)
	manifests, releaseIDs, err := release.DesiredState(r.Context(), s.db, region)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	// If any workload pulls from GHCR, ship a pull secret so the reconciler can
	// pull private images. The rendered Deployments already reference
	// "bordo-registry-creds" for ghcr.io images; here we create that Secret.
	if owner := firstGHCROwner(manifests); owner != "" {
		note := "shipped"
		tok, terr := s.githubHandler.GetScopedPullToken(r.Context())
		switch {
		case terr != nil:
			note = "NOT shipped: " + terr.Error()
		case tok == "":
			note = "NOT shipped: GitHub is not connected (run 'bordo github login')"
		default:
			if secret, serr := fleet.RenderImagePullSecret("bordo-apps", "ghcr.io", owner, tok); serr == nil {
				manifests = secret + "---\n" + manifests
			} else {
				note = "NOT shipped: " + serr.Error()
			}
		}
		s.workloadsMu.Lock()
		if s.pullNotes == nil {
			s.pullNotes = map[string]string{}
		}
		s.pullNotes[region] = note
		s.workloadsMu.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": region, "manifests": manifests, "release_ids": releaseIDs})
}

// handleFleetStatus receives the on-cluster reconciler's report of what it just
// applied (ADR-0006 Phase 2 feedback loop). It is gated by the region's cluster
// token, which also identifies the region. The report advances the named
// releases: on success reconciling→deployed, on an apply error reconciling→failed
// (with the error saved to the release log). The call also counts as a heartbeat.
func (s *Server) handleFleetStatus(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "cluster token required"})
		return
	}
	region, err := fleet.New(s.db).ValidateClusterToken(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid cluster token"})
		return
	}
	var req struct {
		AppliedReleaseIDs []string `json:"applied_release_ids"`
		Error             string   `json:"error"`
		PublicIP          string   `json:"public_ip"`
		Observed          *struct {
			Services []observedSvc `json:"services"`
			Pods     []observedPod `json:"pods"`
		} `json:"observed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	// Reporting is also a liveness signal.
	s.recordRegionPull(r.Context(), region)

	now := time.Now().UTC()
	observed := ""
	if req.Observed != nil {
		rep := workloadReport{Region: region, ReportedAt: now, Services: req.Observed.Services, Pods: req.Observed.Pods}
		// An operator-set public host wins; otherwise fall back to the connection's source
		// address when it is routable.
		var configured string
		_ = s.db.QueryRowContext(r.Context(), `SELECT public_host FROM regions WHERE name = ?`, region).Scan(&configured)
		if configured != "" {
			rep.HostAddr = configured
		} else if publicAddr(req.PublicIP) {
			// Detected by the reconciler on the machine itself, so it sees the real public address.
			rep.HostAddr = req.PublicIP
		} else if host, _, herr := net.SplitHostPort(r.RemoteAddr); herr == nil && publicAddr(host) {
			rep.HostAddr = host
		}
		for _, svc := range rep.Services {
			for _, np := range svc.NodePorts {
				if rep.HostAddr != "" {
					rep.URLs = append(rep.URLs, "http://"+net.JoinHostPort(rep.HostAddr, strconv.Itoa(np)))
				}
			}
		}
		s.workloadsMu.Lock()
		if s.workloads == nil {
			s.workloads = map[string]workloadReport{}
		}
		s.workloads[region] = rep
		s.workloadsMu.Unlock()
		observed = describeWorkloads(rep)
	}
	for _, id := range req.AppliedReleaseIDs {
		if req.Error == "" {
			// Only advance releases still reconciling; never override a release
			// that was rolled back or superseded in the meantime. Scope to the
			// reporting region so a token can't touch another region's releases.
			// "deployed" means the region's reconciler applied the manifests without error. The
			// pull model does not report pod state back, so say that instead of leaving the log empty.
			msg := fmt.Sprintf("[bordo] %s: region %s applied the manifests (kubectl apply succeeded). %s\n",
				now.Format(time.RFC3339), region, observed)
			_, _ = s.db.ExecContext(r.Context(),
				`UPDATE bordo_releases SET status = 'deployed', log = ?, updated_at = ?
				 WHERE id = ? AND region = ? AND status = 'reconciling'`,
				msg, now, id, region)
		} else {
			_, _ = s.db.ExecContext(r.Context(),
				`UPDATE bordo_releases SET status = 'failed', log = ?, updated_at = ?
				 WHERE id = ? AND region = ? AND status = 'reconciling'`,
				req.Error, now, id, region)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded", "region": region})
}

// publicAddr reports whether ip is a routable address worth showing as a URL. A
// loopback, private (Docker bridge, LAN, CGNAT/tailnet) or link-local source means
// the control plane saw a proxy or bridge, not the machine's public address.
func publicAddr(ip string) bool {
	a := net.ParseIP(ip)
	if a == nil {
		return false
	}
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsUnspecified() {
		return false
	}
	if v4 := a.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xC0 == 64 { // 100.64.0.0/10
		return false
	}
	return true
}

// describeWorkloads renders a reconciler snapshot as the human-readable tail of
// a release log: nodePorts and pod states, or an honest "not reported".
func describeWorkloads(rep workloadReport) string {
	var b strings.Builder
	for _, svc := range rep.Services {
		for _, np := range svc.NodePorts {
			host := "<region-host-ip>"
			if rep.HostAddr != "" {
				host = rep.HostAddr
			}
			fmt.Fprintf(&b, "service %s exposes nodePort %d (http://%s:%d). ", svc.Name, np, host, np)
		}
	}
	for _, p := range rep.Pods {
		state := p.Phase
		if p.Ready {
			state += ", ready"
		} else if p.Reason != "" {
			state += ", " + p.Reason
		} else {
			state += ", not ready yet"
		}
		fmt.Fprintf(&b, "pod %s: %s. ", p.Name, state)
	}
	if b.Len() == 0 {
		return "No services or pods were visible in the namespace yet."
	}
	return strings.TrimSpace(b.String())
}

// handleFleetWorkloads returns the latest reconciler-reported services (with
// nodePorts) and pod states, optionally for one region (?region=).
func (s *Server) handleFleetWorkloads(w http.ResponseWriter, r *http.Request) {
	want := r.URL.Query().Get("region")
	s.workloadsMu.RLock()
	out := make([]workloadReport, 0, len(s.workloads))
	for name, rep := range s.workloads {
		if want == "" || want == name {
			rep.PullSecret = s.pullNotes[name]
			out = append(out, rep)
		}
	}
	s.workloadsMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Region < out[j].Region })
	writeJSON(w, http.StatusOK, map[string]any{"regions": out})
}

// recordRegionPull marks a region as live: it stamps last_pull_at, derives the
// stored status to healthy, and refreshes a synthetic "reconciler" heartbeat so
// region_health shows a live node. getHealth/overview downgrade a region whose
// pulls go stale, so this never leaves a dead region reading healthy forever.
func (s *Server) recordRegionPull(ctx context.Context, region string) {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE regions SET status = 'healthy', last_pull_at = ?, updated_at = ? WHERE name = ?`,
		now, now, region); err != nil {
		s.logger.Warn("record region pull", "region", region, "err", err)
		return
	}
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO bordo_node_health (id, region_name, node_name, ready, last_heartbeat, updated_at)
		VALUES (?, ?, 'reconciler', 1, ?, ?)
		ON CONFLICT(region_name, node_name) DO UPDATE SET
			ready = 1, last_heartbeat = excluded.last_heartbeat, updated_at = excluded.updated_at`,
		region+":reconciler", region, now, now)
}

// firstGHCROwner returns the owner from the first ghcr.io/<owner>/... image
// reference in the manifests, or "" if none. Used to build the GHCR pull secret
// without an extra API round-trip.
func firstGHCROwner(manifests string) string {
	i := strings.Index(manifests, "ghcr.io/")
	if i < 0 {
		return ""
	}
	rest := manifests[i+len("ghcr.io/"):]
	j := strings.IndexByte(rest, '/')
	if j < 0 {
		return ""
	}
	return rest[:j]
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

	rows, err := s.db.QueryContext(r.Context(), `SELECT name, status, last_pull_at FROM regions ORDER BY name`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var regions []regionOverview
	for rows.Next() {
		var ro regionOverview
		var lastPull sql.NullTime
		if err := rows.Scan(&ro.Name, &ro.Status, &lastPull); err != nil {
			continue
		}
		// Derive live status from the region's last reconciler pull.
		if lastPull.Valid {
			if time.Since(lastPull.Time) <= fleet.RegionStaleAfter {
				ro.Status = "healthy"
			} else {
				ro.Status = "unreachable"
			}
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
