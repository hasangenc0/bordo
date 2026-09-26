// Package main is the entry point for bordod, the Bordo control-plane daemon.
//
// bordod is a single self-hostable binary that exposes a REST API for the
// Bordo software factory platform. It hosts:
//   - Project registry & catalog (BRD-002)
//   - Fleet state (regions, clusters, VMs) (BRD-011)
//   - Build orchestration (BRD-007)
//   - Release orchestration (BRD-020)
//   - Observe query facade (BRD-031)
//
// State is stored in an embedded SQLite database (~/.bordo/bordod.db) by default,
// or a Postgres database when configured (see bordod.yaml).
//
// Usage:
//
//	bordod serve    [--config <path>] [--port <n>] [--log-level <level>]
//	bordod migrate  [--config <path>]
//	bordod version
//
// See docs/architecture/ADR-0003-control-plane-single-binary.md for rationale.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bordo-io/bordo/control-plane/internal/config"
	"github.com/bordo-io/bordo/control-plane/internal/server"
	"github.com/bordo-io/bordo/control-plane/internal/store"
	"github.com/bordo-io/bordo/control-plane/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "bordod",
		Short: "Bordo control-plane daemon",
		Long: `bordod is the Bordo software factory control plane.

It manages projects, fleet regions, builds, releases, and observability
for the Bordo platform.`,
		SilenceUsage: true,
	}

	root.AddCommand(serveCmd(), migrateCmd(), versionCmd())
	return root
}

// ── serve ─────────────────────────────────────────────────────────────────────

func serveCmd() *cobra.Command {
	var (
		cfgPath   string
		port      int
		logLevel  string
		logFormat string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the bordod API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			if port != 0 {
				cfg.Server.Port = port
			}
			if logLevel != "" {
				cfg.Log.Level = logLevel
			}
			if logFormat != "" {
				cfg.Log.Format = logFormat
			}

			logger := newLogger(cfg.Log)

			db, err := store.Open(cfg.Store)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()

			if err := store.Migrate(db); err != nil {
				return fmt.Errorf("running migrations: %w", err)
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			srv := server.New(cfg, logger, db)
			return srv.Start(ctx)
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "", "path to bordod.yaml (default: auto-discover)")
	cmd.Flags().IntVar(&port, "port", 0, "HTTP port (default: 7401)")
	cmd.Flags().StringVar(&logLevel, "log-level", "", "log level: debug|info|warn|error")
	cmd.Flags().StringVar(&logFormat, "log-format", "", "log format: json|text")
	return cmd
}

// ── migrate ───────────────────────────────────────────────────────────────────

func migrateCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run pending database migrations and exit",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			logger := newLogger(cfg.Log)
			logger.Info("running migrations", "store", cfg.Store.Path)

			db, err := store.Open(cfg.Store)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()

			if err := store.Migrate(db); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			logger.Info("migrations complete")
			return nil
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "", "path to bordod.yaml")
	return cmd
}

// ── version ───────────────────────────────────────────────────────────────────

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.String())
		},
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}
