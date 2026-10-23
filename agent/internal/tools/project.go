package tools

import (
	"context"
	"fmt"
)

func templateListTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "template_list",
		Description: "List all available golden-path project templates with their runtime and description",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		CallFn: func(ctx context.Context, _ map[string]any) (any, error) {
			return cp.Get(ctx, "/v1/templates")
		},
	}
}

func projectCreateTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "project_create",
		Description: "Create a Bordo project, either from a golden-path template (pass 'template') " +
			"or by importing an EXISTING GitHub repository (pass 'repo_url'). Use repo_url whenever the user " +
			"already has a repo; never scaffold a template over it. They are mutually exclusive. " +
			"An imported project is built read-only from the repo's latest commit and needs a root Dockerfile " +
			"and the Bordo build workflow (see project_enable_builds).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     map[string]any{"type": "string", "description": "Project name (kebab-case). Optional when repo_url is given: defaults to the repo name"},
				"template": map[string]any{"type": "string", "description": "Template name, e.g. java-web-service. Omit when importing a repo"},
				"repo_url": map[string]any{"type": "string", "description": "Existing GitHub repo to import, e.g. https://github.com/owner/repo"},
			},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			body := map[string]any{}
			if v, _ := params["name"].(string); v != "" {
				body["name"] = v
			}
			if v, _ := params["template"].(string); v != "" {
				body["template"] = v
			}
			if v, _ := params["repo_url"].(string); v != "" {
				// The control plane field is git_repo_url.
				body["git_repo_url"] = v
			}
			if body["template"] == nil && body["git_repo_url"] == nil {
				return nil, fmt.Errorf("pass either template (new project) or repo_url (existing GitHub repo)")
			}
			if body["name"] == nil && body["git_repo_url"] == nil {
				return nil, fmt.Errorf("name is required")
			}
			return cp.Post(ctx, "/v1/projects", body)
		},
	}
}

func projectEnableBuildsTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "project_enable_builds",
		Description: "Add the Bordo build workflow (.github/workflows/bordo-build.yml) to an imported project's GitHub repo. " +
			"This makes ONE commit on the repo's default branch, so ask the user for permission first and only call it after they agree. " +
			"Needed when a build of an imported project fails with 'no Bordo build workflow'.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project UUID"},
			},
			"required": []string{"project_id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, _ := params["project_id"].(string)
			if id == "" {
				return nil, fmt.Errorf("project_id is required")
			}
			return cp.Post(ctx, "/v1/builds/enable", map[string]any{"project_id": id})
		},
	}
}

func projectCreateRepoTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "project_create_repo",
		Description: "Create the private GitHub repository for a template project and link it, without waiting for a build. " +
			"Use it when a project shows no repo (git_repo_url empty, status pending). Safe to repeat: a project that already has a repo returns it unchanged. " +
			"Fails with a 'GitHub is not connected' message when `bordo github login` has not been run; relay that to the user. " +
			"Not for imported projects: they already point at the user's own repo.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project UUID"},
			},
			"required": []string{"project_id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, _ := params["project_id"].(string)
			if id == "" {
				return nil, fmt.Errorf("project_id is required")
			}
			return cp.Post(ctx, "/v1/builds/repo", map[string]any{"project_id": id})
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
