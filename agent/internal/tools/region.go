package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
		Name: "region_add",
		Description: "Register a new deploy target (region) and mint a one-time join token. " +
			"If the region already exists, it is kept and a fresh token is minted for it (use this when a token expired). " +
			"Returns a command the user runs ON the target machine to join the fleet — " +
			"it installs k3s and connects outbound (no SSH key is ever needed or stored). " +
			"This is the only way to add a machine; there is no SSH-bootstrap path.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Region name, e.g. us-east"},
			},
			"required": []string{"name"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			name, ok := params["name"].(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("name is required")
			}
			// A region that already exists (e.g. the first join token expired) is not an
			// error: fall through and mint a fresh token for it.
			existed := false
			if _, err := cp.Post(ctx, "/v1/regions", map[string]any{"name": name}); err != nil {
				if !strings.Contains(err.Error(), " 409 ") {
					return nil, fmt.Errorf("register region: %w", err)
				}
				existed = true
			}
			raw, err := cp.Post(ctx, "/v1/regions/"+name+"/join-tokens", map[string]any{})
			if err != nil {
				return nil, fmt.Errorf("mint join token: %w", err)
			}
			var tok struct {
				Token     string `json:"token"`
				ExpiresAt string `json:"expires_at"`
			}
			_ = json.Unmarshal(raw, &tok)
			joinCmd := "curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install-node.sh | \\\n" +
				"  BORDO_CP_URL=<control-plane-url-reachable-from-the-machine> \\\n" +
				"  BORDO_JOIN_TOKEN=" + tok.Token + " sh"
			return map[string]any{
				"already_registered": existed,
				"region":             name,
				"expires_at":         tok.ExpiresAt,
				"join_command":       joinCmd,
				"instructions": "Tell the user to run join_command ON the target machine. " +
					"Replace <control-plane-url-reachable-from-the-machine> with the Bordo " +
					"server's address (e.g. http://<server-ip>:7401). The token is single-use " +
					"and expires soon; call region_add again to mint another if needed.",
			}, nil
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
