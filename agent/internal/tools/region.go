package tools

import (
	"context"
	"fmt"
)

func regionListTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "region_list",
		Description: "List all registered fleet regions with their status",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		CallFn: func(ctx context.Context, _ map[string]any) (any, error) {
			return cp.Get(ctx, "/v1/regions")
		},
	}
}

func regionAddTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "region_add",
		Description: "Register a new fleet region (without bootstrapping). Use region_bootstrap to install k3s on a VM.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":       map[string]any{"type": "string", "description": "Region name, e.g. us-east"},
				"kubeconfig": map[string]any{"type": "string", "description": "Optional kubeconfig YAML if the cluster already exists"},
			},
			"required": []string{"name"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			return cp.Post(ctx, "/v1/regions", params)
		},
	}
}

func regionBootstrapTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "region_bootstrap",
		Description: "Bootstrap k3s on a raw VM over SSH and register it as a Bordo region. " +
			"The region must already exist (use region_add first). " +
			"Returns immediately; bootstrapping happens in the background. " +
			"Poll region_list to check when status becomes 'healthy'.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Region name (must already exist)"},
				"host":    map[string]any{"type": "string", "description": "VM IP address or hostname"},
				"port":    map[string]any{"type": "integer", "description": "SSH port (default 22)"},
				"user":    map[string]any{"type": "string", "description": "SSH username (default root)"},
				"ssh_key": map[string]any{"type": "string", "description": "PEM-encoded SSH private key content"},
			},
			"required": []string{"name", "host", "ssh_key"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			name, ok := params["name"].(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("name is required")
			}
			body := map[string]any{
				"host":    params["host"],
				"ssh_key": params["ssh_key"],
			}
			if port, ok := params["port"]; ok {
				body["port"] = port
			}
			if user, ok := params["user"]; ok {
				body["user"] = user
			}
			return cp.Post(ctx, "/v1/regions/"+name+"/bootstrap", body)
		},
	}
}

func regionHealthTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "region_health",
		Description: "Get node-level health for a region (reports from k3s nodes)",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Region name"},
			},
			"required": []string{"name"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			name, ok := params["name"].(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("name is required")
			}
			return cp.Get(ctx, "/v1/regions/"+name+"/health")
		},
	}
}

func fleetOverviewTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "fleet_overview",
		Description: "Get a fleet-wide overview: all regions with their status and latest deployed services",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		CallFn: func(ctx context.Context, _ map[string]any) (any, error) {
			return cp.Get(ctx, "/v1/fleet")
		},
	}
}
