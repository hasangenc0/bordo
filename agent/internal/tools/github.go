package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func githubCreateRepoTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "github_create_repo",
		Description: "Create a GitHub repository and push the scaffolded project source to it. " +
			"Requires a build to have run first so scaffold files exist. " +
			"github_token can also be set via the GITHUB_TOKEN env var.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":   map[string]any{"type": "string", "description": "Bordo project UUID"},
				"repo_name":    map[string]any{"type": "string", "description": "GitHub repository name, e.g. my-service"},
				"github_token": map[string]any{"type": "string", "description": "GitHub personal access token (or set GITHUB_TOKEN env var)"},
				"org":          map[string]any{"type": "string", "description": "GitHub org or user to create the repo under (defaults to the token owner)"},
				"private":      map[string]any{"type": "boolean", "description": "Create a private repo (default: true)"},
			},
			"required": []string{"project_id", "repo_name"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			token, _ := params["github_token"].(string)
			if token == "" {
				token = os.Getenv("GITHUB_TOKEN")
			}
			if token == "" {
				return nil, fmt.Errorf("github_token is required — pass it as a parameter or set GITHUB_TOKEN in the environment")
			}

			projectID, _ := params["project_id"].(string)
			repoName, _ := params["repo_name"].(string)
			org, _ := params["org"].(string)
			private := true
			if v, ok := params["private"].(bool); ok {
				private = v
			}

			// 1. Get project info from control plane.
			projRaw, err := cp.Get(ctx, "/v1/projects/"+projectID)
			if err != nil {
				return nil, fmt.Errorf("fetch project: %w", err)
			}
			var proj map[string]any
			if err := json.Unmarshal(projRaw, &proj); err != nil {
				return nil, fmt.Errorf("parse project: %w", err)
			}

			// 2. Find latest successful build to locate scaffold dir.
			buildsRaw, err := cp.Get(ctx, "/v1/builds?project_id="+projectID)
			if err != nil {
				return nil, fmt.Errorf("fetch builds: %w", err)
			}
			var buildsResp struct {
				Builds []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"builds"`
			}
			_ = json.Unmarshal(buildsRaw, &buildsResp)

			scaffoldDir := ""
			home, _ := os.UserHomeDir()
			for _, b := range buildsResp.Builds {
				if b.Status == "success" {
					candidate := filepath.Join(home, ".bordo", "builds", b.ID, "src")
					if _, err := os.Stat(candidate); err == nil {
						scaffoldDir = candidate
						break
					}
				}
			}
			if scaffoldDir == "" {
				return map[string]any{
					"status":  "no_scaffold",
					"message": "No successful build with scaffold files found. Trigger a build first with build_trigger, then retry.",
				}, nil
			}

			// 3. Create GitHub repo.
			repoURL, err := createGitHubRepo(ctx, token, org, repoName, private)
			if err != nil {
				return nil, fmt.Errorf("create GitHub repo: %w", err)
			}

			// 4. Git init + commit + push.
			if err := gitPush(scaffoldDir, repoURL, token); err != nil {
				return nil, fmt.Errorf("git push: %w — repo created at %s but push failed", err, repoURL)
			}

			return map[string]any{
				"status":   "pushed",
				"repo_url": repoURL,
				"message":  fmt.Sprintf("Scaffolded source pushed to %s", repoURL),
			}, nil
		},
	}
}

func createGitHubRepo(ctx context.Context, token, org, name string, private bool) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"name":        name,
		"private":     private,
		"auto_init":   false,
		"description": "Created by Bordo",
	})

	var apiURL string
	if org != "" {
		apiURL = "https://api.github.com/orgs/" + org + "/repos"
	} else {
		apiURL = "https://api.github.com/user/repos"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		msg, _ := result["message"].(string)
		return "", fmt.Errorf("GitHub API %d: %s", resp.StatusCode, msg)
	}
	cloneURL, _ := result["clone_url"].(string)
	htmlURL, _ := result["html_url"].(string)
	_ = cloneURL
	return htmlURL, nil
}

func gitPush(dir, repoURL, token string) error {
	// Embed token into the push URL for auth.
	// repoURL is https://github.com/org/repo — inject token before github.com.
	authURL := "https://" + token + "@github.com" + repoURL[len("https://github.com"):]

	cmds := [][]string{
		{"git", "-C", dir, "init", "-b", "main"},
		{"git", "-C", dir, "config", "user.email", "bordo@bordo.io"},
		{"git", "-C", dir, "config", "user.name", "Bordo"},
		{"git", "-C", dir, "add", "."},
		{"git", "-C", dir, "commit", "-m", "chore: initial scaffold from Bordo"},
		{"git", "-C", dir, "remote", "add", "origin", authURL},
		{"git", "-C", dir, "push", "-u", "origin", "main"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("command %v: %w\n%s", args[0], err, out)
		}
	}
	return nil
}
