package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const githubAPIBase = "https://api.github.com"

// GenerateJWT creates a JWT signed with the app's RSA private key, valid 10 minutes.
func GenerateJWT(appID int64, privateKeyPEM string) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return "", fmt.Errorf("parse RSA private key: %w", err)
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": strconv.FormatInt(appID, 10),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(key)
}

// GetInstallationToken exchanges a JWT for a short-lived installation access token.
func GetInstallationToken(appJWT string, installationID int64) (string, error) {
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", githubAPIBase, installationID)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", fmt.Errorf("GitHub returned empty installation token (status %d)", resp.StatusCode)
	}
	return result.Token, nil
}

// CreateRepoForInstallation creates a GitHub repository using an installation token.
func CreateRepoForInstallation(ctx context.Context, token, repoName, targetType, targetLogin string, private bool) (cloneURL string, err error) {
	body, _ := json.Marshal(map[string]any{
		"name":        repoName,
		"private":     private,
		"auto_init":   false,
		"description": "Created by Bordo",
	})

	var apiURL string
	if strings.EqualFold(targetType, "Organization") && targetLogin != "" {
		apiURL = fmt.Sprintf("%s/orgs/%s/repos", githubAPIBase, targetLogin)
	} else {
		apiURL = githubAPIBase + "/user/repos"
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
	htmlURL, _ := result["html_url"].(string)
	return htmlURL, nil
}

// ExchangeOAuthCode exchanges a GitHub OAuth code for a user access token.
func ExchangeOAuthCode(clientID, clientSecret, code string) (string, error) {
	params := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	}
	req, err := http.NewRequest(http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("OAuth error: %s — %s", result.Error, result.ErrorDesc)
	}
	return result.AccessToken, nil
}

// CreateRepoForUser creates a repository using a user OAuth token.
func CreateRepoForUser(ctx context.Context, userToken, repoName string, private bool) (string, error) {
	return CreateRepoForInstallation(ctx, userToken, repoName, "User", "", private)
}

// ParseRepoURL extracts owner and repo name from a GitHub HTML URL.
// Accepts: https://github.com/owner/repo  or  git@github.com:owner/repo.git
func ParseRepoURL(rawURL string) (owner, repo string, err error) {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	rawURL = strings.TrimPrefix(rawURL, "git@github.com:")
	rawURL = strings.TrimPrefix(rawURL, "https://github.com/")
	parts := strings.SplitN(rawURL, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("cannot parse GitHub URL %q", rawURL)
	}
	return parts[0], parts[1], nil
}

// PushScaffoldToRepo initialises a git repo in scaffoldDir and force-pushes it
// to the given GitHub repo using an installation access token.
// Returns the HEAD commit SHA.
func PushScaffoldToRepo(ctx context.Context, token, owner, repo, scaffoldDir string) (commitSHA string, err error) {
	remote := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)

	run := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = scaffoldDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %s: %w\n%s", args[0], err, string(out))
		}
		return nil
	}

	steps := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "bordo[bot]@users.noreply.github.com"},
		{"config", "user.name", "Bordo"},
		{"add", "."},
		{"commit", "-m", "Scaffold by Bordo"},
	}
	for _, args := range steps {
		if err := run(args...); err != nil {
			return "", err
		}
	}

	// Capture the commit SHA before pushing (remote may reject the push).
	shaOut, err := exec.CommandContext(ctx, "git", "-C", scaffoldDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	commitSHA = strings.TrimSpace(string(shaOut))

	if err := run("push", remote, "main", "--force"); err != nil {
		return "", err
	}
	return commitSHA, nil
}

// WorkflowRun represents a GitHub Actions workflow run.
type WorkflowRun struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`     // queued | in_progress | completed
	Conclusion string `json:"conclusion"` // success | failure | cancelled | ...
	HTMLURL    string `json:"html_url"`
	HeadSHA    string `json:"head_sha"`
}

// WaitForWorkflowRun polls the GH Actions API until the run for commitSHA
// completes or ctx is cancelled. Poll interval is 15 s.
func WaitForWorkflowRun(ctx context.Context, token, owner, repo, commitSHA string) (*WorkflowRun, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/actions/runs?head_sha=%s&per_page=1",
		githubAPIBase, owner, repo, commitSHA)

	client := &http.Client{Timeout: 15 * time.Second}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// First check immediately, then every 15 s.
	for {
		run, err := fetchLatestRun(ctx, client, token, apiURL)
		if err != nil {
			return nil, err
		}
		if run != nil && run.Status == "completed" {
			return run, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func fetchLatestRun(ctx context.Context, client *http.Client, token, apiURL string) (*WorkflowRun, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		TotalCount   int           `json:"total_count"`
		WorkflowRuns []WorkflowRun `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.WorkflowRuns) == 0 {
		return nil, nil // not triggered yet
	}
	return &result.WorkflowRuns[0], nil
}

// EnsureWorkflowFile pushes a bordo-build.yml workflow to the repo if it doesn't
// already exist on the default branch. Uses the Contents API (no git required).
func EnsureWorkflowFile(ctx context.Context, token, owner, repo string) error {
	const path = ".github/workflows/bordo-build.yml"
	apiURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", githubAPIBase, owner, repo, path)

	client := &http.Client{Timeout: 15 * time.Second}

	// Check if already exists.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil // already present
	}

	body, _ := json.Marshal(map[string]string{
		"message": "Add Bordo build workflow",
		"content": base64.StdEncoding.EncodeToString([]byte(workflowYAML())),
	})
	req2, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/vnd.github+json")
	resp2, err := client.Do(req2)
	if err != nil {
		return err
	}
	resp2.Body.Close()
	if resp2.StatusCode >= 300 {
		return fmt.Errorf("EnsureWorkflowFile: HTTP %d", resp2.StatusCode)
	}
	return nil
}

// workflowYAML returns the canonical bordo-build.yml content.
func workflowYAML() string {
	return `name: Bordo Build

on:
  push:
    branches: [main]
  workflow_dispatch:

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: |
            ghcr.io/${{ github.repository }}:${{ github.sha }}
            ghcr.io/${{ github.repository }}:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
`
}

// GetCurrentUserLogin returns the GitHub login of the authenticated user.
func GetCurrentUserLogin(userToken string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPIBase+"/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Login string `json:"login"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result.Login, nil
}

