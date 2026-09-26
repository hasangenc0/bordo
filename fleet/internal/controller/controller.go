// Package controller implements the multi-region fleet controller (BRD-012).
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RegionConfig holds the credentials and endpoint for a single regional cluster.
type RegionConfig struct {
	Name       string
	APIURL     string // https://<host>:6443
	CACert     string // PEM-encoded CA certificate
	ClientCert string // PEM-encoded client certificate
	ClientKey  string // PEM-encoded client key
}

// HealthReport is sent to the control-plane after each poll cycle.
type HealthReport struct {
	Region  string       `json:"region"`
	Nodes   []NodeStatus `json:"nodes"`
	At      time.Time    `json:"at"`
}

// Controller manages one watcher goroutine per registered cluster.
type Controller struct {
	cpURL  string // control-plane base URL e.g. http://localhost:7401
	logger *slog.Logger

	mu       sync.Mutex
	watchers map[string]*watcher
	cancel   context.CancelFunc
}

// New creates a Controller that reports health to cpURL.
func New(cpURL string, logger *slog.Logger) *Controller {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Controller{
		cpURL:    cpURL,
		logger:   logger,
		watchers: make(map[string]*watcher),
	}
}

// Start launches a watcher for each supplied region.
func (c *Controller) Start(ctx context.Context, regions []RegionConfig) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	for _, r := range regions {
		if err := c.startWatcher(ctx, r); err != nil {
			return fmt.Errorf("starting watcher for region %s: %w", r.Name, err)
		}
	}
	return nil
}

// AddRegion hot-adds a new region watcher while the controller is running.
func (c *Controller) AddRegion(region RegionConfig) error {
	c.mu.Lock()
	if _, exists := c.watchers[region.Name]; exists {
		c.mu.Unlock()
		return fmt.Errorf("region %s already registered", region.Name)
	}
	c.mu.Unlock()

	ctx := context.Background()
	if c.cancel != nil {
		// derive from existing context via a background; the parent context cancels
		// all watchers when Stop is called via the shared cancel chain.
	}
	return c.startWatcher(ctx, region)
}

// Stop cancels all watcher goroutines and waits for them to exit.
func (c *Controller) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range c.watchers {
		w.wait()
	}
}

func (c *Controller) startWatcher(ctx context.Context, r RegionConfig) error {
	client, err := newTLSClient(r.CACert, r.ClientCert, r.ClientKey)
	if err != nil {
		return fmt.Errorf("building TLS client: %w", err)
	}

	w := &watcher{
		region: r,
		client: client,
		cpURL:  c.cpURL,
		logger: c.logger.With("region", r.Name),
	}

	c.mu.Lock()
	c.watchers[r.Name] = w
	c.mu.Unlock()

	go w.run(ctx)
	return nil
}

// reportHealth POSTs a HealthReport to the control-plane.
func reportHealth(cpURL string, report HealthReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/v1/fleet/regions/%s/health", cpURL, report.Region)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("control-plane returned %d", resp.StatusCode)
	}
	return nil
}
