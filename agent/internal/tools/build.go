package tools

import (
	"context"
	"fmt"
)

func buildTriggerTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "build_trigger",
		Description: "Trigger a container build for a project",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project UUID"},
				"git_ref":    map[string]any{"type": "string", "description": "Git ref to build (branch, tag, or SHA)"},
			},
			"required": []string{"project_id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			return cp.Post(ctx, "/v1/builds", params)
		},
	}
}

func buildStatusTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "build_status",
		Description: "Get the status of a build by ID",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Build UUID"},
			},
			"required": []string{"id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, ok := params["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("id is required")
			}
			return cp.Get(ctx, "/v1/builds/"+id)
		},
	}
}

func buildLogsTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "build_logs",
		Description: "Get the build logs for a build",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Build UUID"},
			},
			"required": []string{"id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, ok := params["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("id is required")
			}
			return cp.Get(ctx, "/v1/builds/"+id+"/logs")
		},
	}
}
