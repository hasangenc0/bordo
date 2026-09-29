package tools

import (
	"context"
	"fmt"
)

func deployListTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "deploy_list",
		Description: "List recent deployments/releases, optionally filtered by project",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Filter by project UUID (optional)"},
			},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			path := "/v1/releases"
			if pid, ok := params["project_id"].(string); ok && pid != "" {
				path += "?project_id=" + pid
			}
			return cp.Get(ctx, path)
		},
	}
}

func deployTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "deploy",
		Description: "Deploy an image to one or more regions. " +
			"Use 'regions' for multi-region deployment (e.g. deploy web-service-a to us-east, eu-west, and ap-south at once). " +
			"Use 'region' for a single region. Returns a release_group_id when deploying to multiple regions.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project UUID"},
				"image_tag":  map[string]any{"type": "string", "description": "OCI image tag to deploy, e.g. ghcr.io/org/repo:sha"},
				"region":     map[string]any{"type": "string", "description": "Single target region name (use 'regions' for multi-region)"},
				"regions": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "List of region names for multi-region deployment",
				},
				"strategy": map[string]any{
					"type":        "string",
					"description": "Rollout strategy: rolling, canary, or blue-green",
					"enum":        []string{"rolling", "canary", "blue-green"},
				},
			},
			"required": []string{"project_id", "image_tag"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			return cp.Post(ctx, "/v1/releases", params)
		},
	}
}

func deployStatusTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "deploy_status",
		Description: "Get the status of a single release by ID",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Release UUID"},
			},
			"required": []string{"id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, ok := params["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("id is required")
			}
			return cp.Get(ctx, "/v1/releases/"+id)
		},
	}
}

func deployGroupStatusTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "deploy_group_status",
		Description: "Get the status of all releases in a multi-region deployment group",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"group_id": map[string]any{"type": "string", "description": "release_group_id returned by a multi-region deploy"},
			},
			"required": []string{"group_id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			groupID, ok := params["group_id"].(string)
			if !ok || groupID == "" {
				return nil, fmt.Errorf("group_id is required")
			}
			return cp.Get(ctx, "/v1/releases/group/"+groupID)
		},
	}
}

func deployLogsTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "deploy_logs",
		Description: "Get the kubectl apply and rollout output for a release",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Release UUID"},
			},
			"required": []string{"id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, ok := params["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("id is required")
			}
			return cp.Get(ctx, "/v1/releases/"+id+"/logs")
		},
	}
}

func rollbackTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "rollback",
		Description: "Roll back a release to the previous stable version",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Release UUID to roll back"},
			},
			"required": []string{"id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, ok := params["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("id is required")
			}
			return cp.Post(ctx, "/v1/releases/"+id+"/rollback", nil)
		},
	}
}
