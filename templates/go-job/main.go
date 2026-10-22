package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// How often to run the job. Override with BORDO_JOB_INTERVAL, e.g. "30s", "5m", "1h".
	interval := 60 * time.Second
	if v := os.Getenv("BORDO_JOB_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		} else {
			logger.Warn("invalid BORDO_JOB_INTERVAL, using default", "value", v, "default", interval.String())
		}
	}

	// A small health server so Bordo's readiness probe has a target.
	go serveHealth(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	logger.Info("{{.ProjectName}} job runner started", "interval", interval.String())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once at startup, then on every tick until shutdown.
	runJob(logger)
	for {
		select {
		case <-ticker.C:
			runJob(logger)
		case <-ctx.Done():
			logger.Info("shutting down")
			return
		}
	}
}

// runJob is where your scheduled work goes.
func runJob(logger *slog.Logger) {
	start := time.Now()
	logger.Info("[{{.ProjectName}}] executing job")

	// TODO: implement your job logic here.

	logger.Info("[{{.ProjectName}}] job finished", "dur", time.Since(start).String())
}

func serveHealth(logger *slog.Logger) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	ok := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", ok)
	mux.HandleFunc("GET /actuator/health", ok)

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("health server error", "err", err)
	}
}
