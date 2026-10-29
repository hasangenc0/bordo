package tools

import (
	"context"
	"errors"
	"net/url"
)

func autodeploySetTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "autodeploy_set",
		Description: "Turn on deploy-on-merge for a project: when its repository's default branch gets a new commit, Bordo " +
			"builds it and deploys it to the region, with no further prompting. By default each deploy waits for the " +
			"user's approval (an approval card); pass require_approval=false only when the user explicitly asks for " +
			"unattended deploys. Code already on the default branch is not redeployed when this is enabled.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":       map[string]any{"type": "string", "description": "Project id or exact name"},
				"region":           map[string]any{"type": "string", "description": "Region to deploy to"},
				"expose":           map[string]any{"type": "string", "enum": []string{"none", "nodeport", "ingress"}, "description": "How to expose the app (default none)"},
				"require_approval": map[string]any{"type": "boolean", "description": "Ask the user before each deploy (default true)"},
			},
			"required": []string{"project_id", "region"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id := str(params, "project_id")
			if id == "" {
				return nil, errors.New("project_id is required")
			}
			body := map[string]any{"region": str(params, "region"), "expose": str(params, "expose")}
			if v, ok := params["require_approval"].(bool); ok {
				body["require_approval"] = v
			}
			return cp.Put(ctx, "/v1/projects/"+url.PathEscape(id)+"/autodeploy", body)
		},
	}
}

func autodeployStatusTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "autodeploy_status",
		Description: "Show whether deploy-on-merge is on for a project, where it deploys, the last commit it handled and what happened to it (deployed, waiting for approval, build failed, rejected).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project id or exact name"},
			},
			"required": []string{"project_id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id := str(params, "project_id")
			if id == "" {
				return nil, errors.New("project_id is required")
			}
			return cp.Get(ctx, "/v1/projects/"+url.PathEscape(id)+"/autodeploy")
		},
	}
}

func autodeployOffTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "autodeploy_off",
		Description: "Turn deploy-on-merge off for a project.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project id or exact name"},
			},
			"required": []string{"project_id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id := str(params, "project_id")
			if id == "" {
				return nil, errors.New("project_id is required")
			}
			return cp.Delete(ctx, "/v1/projects/"+url.PathEscape(id)+"/autodeploy")
		},
	}
}
