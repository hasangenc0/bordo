package tools

import (
	"context"
	"fmt"
)

func projectCreateTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "project_create",
		Description: "Create a new Bordo project from a golden-path template",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "Project name (kebab-case)"},
				"description": map[string]any{"type": "string", "description": "Short description"},
				"template":    map[string]any{"type": "string", "description": "Template name, e.g. java-web-service"},
			},
			"required": []string{"name", "template"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			return cp.Post(ctx, "/v1/projects", params)
		},
	}
}

func projectListTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "project_list",
		Description: "List all projects registered in the Bordo catalog",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		CallFn: func(ctx context.Context, _ map[string]any) (any, error) {
			return cp.Get(ctx, "/v1/projects")
		},
	}
}

func projectGetTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "project_get",
		Description: "Get details of a specific project by ID",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Project UUID"},
			},
			"required": []string{"id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, ok := params["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("id is required")
			}
			return cp.Get(ctx, "/v1/projects/"+id)
		},
	}
}

func projectDeleteTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "project_delete",
		Description: "Delete a project by ID",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Project UUID"},
			},
			"required": []string{"id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, ok := params["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("id is required")
			}
			return cp.Delete(ctx, "/v1/projects/"+id)
		},
	}
}
