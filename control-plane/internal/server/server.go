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
	"time"

	"github.com/bordo-io/bordo/control-plane/internal/buildorchestrator"
	"github.com/bordo-io/bordo/control-plane/internal/config"
	"github.com/bordo-io/bordo/control-plane/internal/fleet"
	"github.com/bordo-io/bordo/control-plane/internal/observe"
	"github.com/bordo-io/bordo/control-plane/internal/registry"
	"github.com/bordo-io/bordo/control-plane/internal/release"
	"github.com/bordo-io/bordo/control-plane/internal/secrets"
	"github.com/bordo-io/bordo/control-plane/internal/store"
	"github.com/bordo-io/bordo/control-plane/internal/version"
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
}

// New creates a Server wired with all routes.
func New(cfg *config.Config, logger *slog.Logger, db *sql.DB) *Server {
	secretKey, err := secrets.DeriveKey()
	if err != nil {
		// Non-fatal: log the error and use the zero key (secrets will be unusable but server still starts).
		logger.Warn("secrets: could not derive encryption key, secrets disabled", "error", err)
	}
	s := &Server{
		cfg:               cfg,
		logger:            logger,
		db:                db,
		router:            chi.NewRouter(),
		secretKey:         secretKey,
		releaseController: release.NewController(db, logger),
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

// registerRoutes wires all API routes.
func (s *Server) registerRoutes() {
	// Health / readiness probes (used by k8s and load balancers).
	s.router.Get("/healthz", s.handleHealthz)
	s.router.Get("/readyz", s.handleReadyz)
	s.router.Get("/version", s.handleVersion)

	// Versioned API — route groups hang off /v1.
	s.router.Route("/v1", func(r chi.Router) {
		r.Mount("/projects", registry.NewHandler(registry.New(s.db)))
		r.Mount("/regions", fleet.NewHandler(fleet.New(s.db)))
		buildStore := buildorchestrator.New(s.db)
		buildExecutor := buildorchestrator.NewExecutor(buildStore, s.db, s.logger, "", "", "")
		r.Mount("/builds", buildorchestrator.NewHandler(buildStore, buildExecutor))
		r.Mount("/releases", release.NewHandler(s.db))

		secretsHandler := secrets.NewHandler(secrets.NewSQLiteStore(s.db, s.secretKey))
		r.Route("/projects/{projectID}/secrets", func(r chi.Router) {
			r.Post("/", secretsHandler.Set)
			r.Get("/", secretsHandler.List)
			r.Delete("/{key}", secretsHandler.Delete)
		})

		observeHandler := observe.NewHandler(s.db)
		r.Get("/observe/metrics", observeHandler.QueryMetrics)
		r.Get("/observe/logs", observeHandler.QueryLogs)
		r.Get("/observe/traces/{traceID}", observeHandler.QueryTrace)

		r.Get("/templates", s.handleListTemplates)
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
