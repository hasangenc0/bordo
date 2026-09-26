package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// RegionBackend holds observe backend URLs for one region.
type RegionBackend struct {
	Region   string
	VMURL    string
	LokiURL  string
	TempoURL string
}

// MultiClient fans queries out to all registered regions and merges the results.
type MultiClient struct {
	backends []RegionBackend
	http     *http.Client
}

// NewMultiClient creates a MultiClient.
func NewMultiClient(backends []RegionBackend) *MultiClient {
	return &MultiClient{
		backends: backends,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

type regionResult struct {
	region string
	data   json.RawMessage
	err    error
}

// QueryMetrics queries all regions (or a specific one) for a PromQL expression.
// Returns a map of region → raw JSON response.
func (m *MultiClient) QueryMetrics(ctx context.Context, promql, region string) (map[string]json.RawMessage, error) {
	targets := m.filter(region)
	if len(targets) == 0 {
		return nil, fmt.Errorf("no backends configured (deploy the observe stack and set vm_url per region)")
	}
	results := make(chan regionResult, len(targets))
	var wg sync.WaitGroup
	for _, b := range targets {
		if b.VMURL == "" {
			continue
		}
		wg.Add(1)
		go func(b RegionBackend) {
			defer wg.Done()
			params := url.Values{
				"query": {promql},
				"time":  {fmt.Sprintf("%d", time.Now().Unix())},
			}
			data, err := m.get(ctx, b.VMURL+"/api/v1/query?"+params.Encode())
			results <- regionResult{region: b.Region, data: data, err: err}
		}(b)
	}
	go func() { wg.Wait(); close(results) }()
	return collect(results), nil
}

// QueryLogs queries all regions (or a specific one) for a LogQL expression.
func (m *MultiClient) QueryLogs(ctx context.Context, logql, region string, start, end time.Time) (map[string]json.RawMessage, error) {
	targets := m.filter(region)
	if len(targets) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}
	results := make(chan regionResult, len(targets))
	var wg sync.WaitGroup
	for _, b := range targets {
		if b.LokiURL == "" {
			continue
		}
		wg.Add(1)
		go func(b RegionBackend) {
			defer wg.Done()
			params := url.Values{
				"query": {logql},
				"start": {fmt.Sprintf("%d", start.UnixNano())},
				"end":   {fmt.Sprintf("%d", end.UnixNano())},
				"limit": {"100"},
			}
			data, err := m.get(ctx, b.LokiURL+"/loki/api/v1/query_range?"+params.Encode())
			results <- regionResult{region: b.Region, data: data, err: err}
		}(b)
	}
	go func() { wg.Wait(); close(results) }()
	return collect(results), nil
}

// QueryTrace fetches a trace by ID from all regions (or a specific one); returns first match.
func (m *MultiClient) QueryTrace(ctx context.Context, traceID, region string) (json.RawMessage, error) {
	targets := m.filter(region)
	if len(targets) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}
	for _, b := range targets {
		if b.TempoURL == "" {
			continue
		}
		data, err := m.get(ctx, b.TempoURL+"/api/traces/"+traceID)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}
	return json.RawMessage(`{"status":"not_found"}`), nil
}

// filter returns the backends matching the given region name, or all if region is "".
func (m *MultiClient) filter(region string) []RegionBackend {
	if region == "" {
		return m.backends
	}
	for _, b := range m.backends {
		if b.Region == region {
			return []RegionBackend{b}
		}
	}
	return nil
}

// get performs a GET and returns the response body.
func (m *MultiClient) get(ctx context.Context, u string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// collect drains the results channel into a map.
func collect(ch <-chan regionResult) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage)
	for r := range ch {
		if r.err != nil {
			out[r.region] = json.RawMessage(fmt.Sprintf(`{"error":%q}`, r.err.Error()))
		} else {
			out[r.region] = r.data
		}
	}
	return out
}
