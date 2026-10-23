package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const probeMaxBody = 8 << 10

// appProbeTool does a read-only GET against an app Bordo deployed. It can only reach
// http://<region host>:<nodePort> pairs the region's reconciler reported, so it cannot
// be pointed at arbitrary or internal addresses.
func appProbeTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "app_probe",
		Description: "Send a read-only GET request to a deployed app's NodePort and return the status code, content type and the start of the body. " +
			"Use it to verify an app actually serves (e.g. path /healthz or /api/v1/events). " +
			"Only works for a region whose public host is set (region_set_host) and a nodePort reported by app_endpoints.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"region":    map[string]any{"type": "string", "description": "Region name"},
				"node_port": map[string]any{"type": "integer", "description": "NodePort from app_endpoints (optional when the region has exactly one)"},
				"path":      map[string]any{"type": "string", "description": "Request path and query, e.g. /api/v1/events?limit=5 (default /)"},
			},
			"required": []string{"region"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			region, _ := params["region"].(string)
			if region == "" {
				return nil, fmt.Errorf("region is required")
			}
			raw, err := cp.Get(ctx, "/v1/fleet/workloads?region="+url.QueryEscape(region))
			if err != nil {
				return nil, err
			}
			port := 0
			if f, ok := params["node_port"].(float64); ok {
				port = int(f)
			}
			path, _ := params["path"].(string)
			target, err := probeTarget(raw, region, port, path)
			if err != nil {
				return nil, err
			}
			return probeGet(ctx, &http.Client{Timeout: 10 * time.Second}, target)
		},
	}
}

// probeTarget builds the URL to probe from a /v1/fleet/workloads response, allowing only
// a reported (host, nodePort) pair.
func probeTarget(workloads []byte, region string, port int, path string) (string, error) {
	var resp struct {
		Regions []struct {
			Region   string `json:"region"`
			HostAddr string `json:"host_addr"`
			Services []struct {
				NodePorts []int `json:"node_ports"`
			} `json:"services"`
		} `json:"regions"`
	}
	if err := json.Unmarshal(workloads, &resp); err != nil {
		return "", err
	}
	for _, r := range resp.Regions {
		if r.Region != region {
			continue
		}
		if r.HostAddr == "" {
			return "", fmt.Errorf("region %q has no known public host: ask the user for it and save it with region_set_host", region)
		}
		var ports []int
		for _, s := range r.Services {
			ports = append(ports, s.NodePorts...)
		}
		if port == 0 {
			if len(ports) != 1 {
				return "", fmt.Errorf("region %q reports nodePorts %v: pass node_port", region, ports)
			}
			port = ports[0]
		}
		ok := false
		for _, p := range ports {
			ok = ok || p == port
		}
		if !ok {
			return "", fmt.Errorf("nodePort %d is not reported for region %q (reported: %v)", port, region, ports)
		}
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		u := url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", r.HostAddr, port)}
		if strings.Contains(r.HostAddr, ":") { // IPv6
			u.Host = fmt.Sprintf("[%s]:%d", r.HostAddr, port)
		}
		return u.String() + path, nil
	}
	return "", fmt.Errorf("region %q has not reported workloads yet (the reconciler reports about every 30s)", region)
}

func probeGet(ctx context.Context, c *http.Client, target string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s failed: %w (the port may be closed in the firewall, or unreachable from the Bordo server)", target, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, probeMaxBody+1))
	truncated := len(body) > probeMaxBody
	if truncated {
		body = body[:probeMaxBody]
	}
	return map[string]any{
		"url":          target,
		"status":       resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"elapsed_ms":   time.Since(start).Milliseconds(),
		"body":         string(body),
		"truncated":    truncated,
	}, nil
}
