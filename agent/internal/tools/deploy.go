package tools

import (
	"context"
	"fmt"
)

func deployTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "deploy",
		Description: "Deploy an image to a region using a given strategy",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project UUID"},
				"image_tag":  map[string]any{"type": "string", "description": "OCI image tag to deploy"},
				"region":     map[string]any{"type": "string", "description": "Target region name"},
				"strategy": map[string]any{
					"type":        "string",
					"description": "Rollout strategy: rolling, canary, or blue-green",
					"enum":        []string{"rolling", "canary", "blue-green"},
				},
			},
			"required": []string{"project_id", "image_tag", "region"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			return cp.Post(ctx, "/v1/releases", params)
		},
	}
}

func deployStatusTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "deploy_status",
		Description: "Get the status of a release/deploy by ID",
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
