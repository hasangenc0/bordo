package tools

import "context"

func regionListTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "region_list",
		Description: "List all registered fleet regions",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		CallFn: func(ctx context.Context, _ map[string]any) (any, error) {
			return cp.Get(ctx, "/v1/fleet/regions")
		},
	}
}

func regionAddTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "region_add",
		Description: "Register a new fleet region and bootstrap k3s on the target host",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string", "description": "Region name, e.g. us-east-1"},
				"api_url":      map[string]any{"type": "string", "description": "k3s API server URL after bootstrap"},
				"ssh_host":     map[string]any{"type": "string", "description": "SSH host for the bootstrap node"},
				"ssh_user":     map[string]any{"type": "string", "description": "SSH username"},
				"ssh_key_path": map[string]any{"type": "string", "description": "Path to SSH private key"},
			},
			"required": []string{"name", "ssh_host", "ssh_user", "ssh_key_path"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			return cp.Post(ctx, "/v1/fleet/regions", params)
		},
	}
}
