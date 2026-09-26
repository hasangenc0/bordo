package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	pollInterval   = 30 * time.Second
	maxBackoff     = 5 * time.Minute
	maxRetries     = 5
	initialBackoff = 5 * time.Second
)

// NodeStatus represents the health of a single k3s node.
type NodeStatus struct {
	Name          string    `json:"name"`
	Ready         bool      `json:"ready"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
}

type watcher struct {
	region RegionConfig
	client *http.Client
	cpURL  string
	logger *slog.Logger

	done chan struct{}
	once sync.Once
}

func (w *watcher) wait() {
	if w.done != nil {
		<-w.done
	}
}

func (w *watcher) run(ctx context.Context) {
	w.done = make(chan struct{})
	defer close(w.done)

	w.logger.Info("fleet watcher starting")
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Poll immediately on start.
	w.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("fleet watcher stopped")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *watcher) poll(ctx context.Context) {
	nodes, err := w.fetchNodes(ctx)
	if err != nil {
		w.logger.Error("failed to fetch nodes; will retry with backoff", "err", err)
		w.reconnectWithBackoff(ctx)
		return
	}

	report := HealthReport{
		Region: w.region.Name,
		Nodes:  nodes,
		At:     time.Now().UTC(),
	}
	if err := reportHealth(w.cpURL, report); err != nil {
		w.logger.Warn("could not report health to control-plane", "err", err)
	}
}

// fetchNodes retrieves node statuses from the k3s API server.
func (w *watcher) fetchNodes(ctx context.Context) ([]NodeStatus, error) {
	url := w.region.APIURL + "/api/v1/nodes"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("k3s API returned %d", resp.StatusCode)
	}

	var nodeList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type               string `json:"type"`
					Status             string `json:"status"`
					LastHeartbeatTime  string `json:"lastHeartbeatTime"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&nodeList); err != nil {
		return nil, fmt.Errorf("decoding node list: %w", err)
	}

	statuses := make([]NodeStatus, 0, len(nodeList.Items))
	for _, item := range nodeList.Items {
		ns := NodeStatus{Name: item.Metadata.Name}
		for _, cond := range item.Status.Conditions {
			if cond.Type == "Ready" {
				ns.Ready = cond.Status == "True"
				if t, err := time.Parse(time.RFC3339, cond.LastHeartbeatTime); err == nil {
					ns.LastHeartbeat = t
				}
				break
			}
		}
		statuses = append(statuses, ns)
	}
	return statuses, nil
}

// reconnectWithBackoff retries up to maxRetries with exponential backoff.
// After maxRetries failures, it waits maxBackoff before the next attempt.
func (w *watcher) reconnectWithBackoff(ctx context.Context) {
	backoff := initialBackoff
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		nodes, err := w.fetchNodes(ctx)
		if err == nil {
			w.logger.Info("reconnected to cluster", "attempt", attempt)
			report := HealthReport{Region: w.region.Name, Nodes: nodes, At: time.Now().UTC()}
			if err := reportHealth(w.cpURL, report); err != nil {
				w.logger.Warn("could not report health after reconnect", "err", err)
			}
			return
		}
		w.logger.Warn("reconnect attempt failed", "attempt", attempt, "err", err)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	// All attempts exhausted; wait maxBackoff and let the normal ticker retry.
	w.logger.Error("cluster unreachable after max retries; backing off", "wait", maxBackoff)
	select {
	case <-ctx.Done():
	case <-time.After(maxBackoff):
	}
}

// newTLSClient builds an http.Client from PEM-encoded certificate strings.
// If all cert strings are empty, it returns a default client (useful for
// clusters using token-based auth or insecure dev setups).
func newTLSClient(caCert, clientCert, clientKey string) (*http.Client, error) {
	tlsCfg := &tls.Config{}

	if caCert != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caCert)) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsCfg.RootCAs = pool
	}

	if clientCert != "" && clientKey != "" {
		cert, err := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
		if err != nil {
			return nil, fmt.Errorf("parsing client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{TLSClientConfig: tlsCfg}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}, nil
}
