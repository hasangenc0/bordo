// Package query provides HTTP clients for querying the Bordo observability backends.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Backend holds connection URLs for the three observe backends.
type Backend struct {
	VictoriaMetricsURL string
	LokiURL            string
	TempoURL           string
}

// MetricsResult is the response from a PromQL instant query.
type MetricsResult struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// Client queries VictoriaMetrics, Loki, and Tempo.
type Client struct {
	backend Backend
	http    *http.Client
}

// NewClient creates a Client with a 30-second timeout.
func NewClient(backend Backend) *Client {
	return &Client{
		backend: backend,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// QueryMetrics executes a PromQL instant query against VictoriaMetrics.
// GET <vm>/api/v1/query?query=<promql>&time=<unix>
func (c *Client) QueryMetrics(ctx context.Context, promql string) (*MetricsResult, error) {
	if c.backend.VictoriaMetricsURL == "" {
		return &MetricsResult{Status: "backends_not_configured"}, nil
	}
	params := url.Values{"query": {promql}, "time": {fmt.Sprintf("%d", time.Now().Unix())}}
	endpoint := c.backend.VictoriaMetricsURL + "/api/v1/query?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result MetricsResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode metrics response: %w", err)
	}
	return &result, nil
}

// QueryLogs executes a LogQL range query against Loki.
// GET <loki>/loki/api/v1/query_range?query=<logql>&start=<>&end=<>&limit=100
func (c *Client) QueryLogs(ctx context.Context, logql string, start, end time.Time) (json.RawMessage, error) {
	if c.backend.LokiURL == "" {
		return json.RawMessage(`{"status":"backends_not_configured"}`), nil
	}
	params := url.Values{
		"query": {logql},
		"start": {fmt.Sprintf("%d", start.UnixNano())},
		"end":   {fmt.Sprintf("%d", end.UnixNano())},
		"limit": {"100"},
	}
	endpoint := c.backend.LokiURL + "/loki/api/v1/query_range?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// QueryTrace fetches a trace from Tempo by trace ID.
// GET <tempo>/api/traces/<traceID>
func (c *Client) QueryTrace(ctx context.Context, traceID string) (json.RawMessage, error) {
	if c.backend.TempoURL == "" {
		return json.RawMessage(`{"status":"backends_not_configured"}`), nil
	}
	endpoint := c.backend.TempoURL + "/api/traces/" + traceID

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
