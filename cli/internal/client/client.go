// Package client provides an HTTP client for the bordod REST API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/hasangenc0/bordo/cli/internal/config"
)

// Client is a typed HTTP client for the bordod REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewFromConfig creates a Client from the on-disk CLI config.
// Priority: env vars > config file > defaults.
func NewFromConfig() (*Client, error) {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{Server: "http://localhost:7401"}
	}
	// Env var overrides take precedence.
	if v := os.Getenv("BORDO_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("BORDO_TOKEN"); v != "" {
		cfg.Token = v
	}
	return New(cfg.Server, cfg.Token), nil
}

// Get calls GET <path> and returns the raw response body.
func (c *Client) Get(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	c.addAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// New creates a Client targeting baseURL, authenticating with token.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Project is the control-plane project model returned by the API.
type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Template   string `json:"template"`
	GitRepoURL string `json:"git_repo_url"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// CreateProject calls POST /v1/projects.
func (c *Client) CreateProject(ctx context.Context, name, template string) (*Project, error) {
	body := map[string]string{"name": name, "template": template}
	var p Project
	if err := c.post(ctx, "/v1/projects", body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProjects calls GET /v1/projects.
func (c *Client) ListProjects(ctx context.Context) ([]*Project, error) {
	var resp struct {
		Projects []*Project `json:"projects"`
	}
	if err := c.get(ctx, "/v1/projects", &resp); err != nil {
		return nil, err
	}
	return resp.Projects, nil
}

// GetProject calls GET /v1/projects/{id}.
func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var p Project
	if err := c.get(ctx, "/v1/projects/"+id, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Build is a container build record.
type Build struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Status       string `json:"status"`
	ImageName    string `json:"image_name"`
	ImageTag     string `json:"image_tag"`
	ImageDigest  string `json:"image_digest"`
	ErrorMessage string `json:"error_message"`
	CreatedAt    string `json:"created_at"`
}

// TriggerBuild calls POST /v1/builds.
func (c *Client) TriggerBuild(ctx context.Context, projectID, imageName, imageTag, registryURL string) (*Build, error) {
	body := map[string]string{
		"project_id":   projectID,
		"image_name":   imageName,
		"image_tag":    imageTag,
		"registry_url": registryURL,
	}
	var b Build
	if err := c.post(ctx, "/v1/builds", body, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBuilds calls GET /v1/builds?project_id=<id>.
func (c *Client) ListBuilds(ctx context.Context, projectID string) ([]*Build, error) {
	var resp struct {
		Builds []*Build `json:"builds"`
	}
	if err := c.get(ctx, "/v1/builds?project_id="+projectID, &resp); err != nil {
		return nil, err
	}
	return resp.Builds, nil
}

// GetBuildLogs calls GET /v1/builds/{id}/logs.
func (c *Client) GetBuildLogs(ctx context.Context, buildID string) ([]string, error) {
	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := c.get(ctx, "/v1/builds/"+buildID+"/logs", &resp); err != nil {
		return nil, err
	}
	return resp.Lines, nil
}

// DeleteProject calls DELETE /v1/projects/{id}.
// SecretEntry is a secret key record (value is never returned).
type SecretEntry struct {
	Key       string `json:"key"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// SetSecret calls POST /v1/projects/{projectID}/secrets.
func (c *Client) SetSecret(ctx context.Context, projectID, key, value string) error {
	body := map[string]string{"key": key, "value": value}
	var out map[string]string
	return c.post(ctx, "/v1/projects/"+projectID+"/secrets", body, &out)
}

// ListSecrets calls GET /v1/projects/{projectID}/secrets.
func (c *Client) ListSecrets(ctx context.Context, projectID string) ([]SecretEntry, error) {
	var resp struct {
		Secrets []SecretEntry `json:"secrets"`
	}
	if err := c.get(ctx, "/v1/projects/"+projectID+"/secrets", &resp); err != nil {
		return nil, err
	}
	return resp.Secrets, nil
}

// DeleteSecret calls DELETE /v1/projects/{projectID}/secrets/{key}.
func (c *Client) DeleteSecret(ctx context.Context, projectID, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/v1/projects/"+projectID+"/secrets/"+key, nil)
	if err != nil {
		return err
	}
	c.addAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return c.readError(resp)
}

// Release is a deployment record.
type Release struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	ImageTag  string `json:"image_tag"`
	Region    string `json:"region"`
	Strategy  string `json:"strategy"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CreateRelease calls POST /v1/releases.
func (c *Client) CreateRelease(ctx context.Context, projectID, imageTag, region, strategy string) (*Release, error) {
	body := map[string]string{
		"project_id": projectID,
		"image_tag":  imageTag,
		"region":     region,
		"strategy":   strategy,
	}
	var rel Release
	if err := c.post(ctx, "/v1/releases", body, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// GetRelease calls GET /v1/releases/{id}.
func (c *Client) GetRelease(ctx context.Context, id string) (*Release, error) {
	var rel Release
	if err := c.get(ctx, "/v1/releases/"+id, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// RollbackRelease calls POST /v1/releases/{id}/rollback.
func (c *Client) RollbackRelease(ctx context.Context, id string) (*Release, error) {
	var rel Release
	if err := c.post(ctx, "/v1/releases/"+id+"/rollback", nil, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func (c *Client) DeleteProject(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/v1/projects/"+id, nil)
	if err != nil {
		return err
	}
	c.addAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return c.readError(resp)
}

// GetRaw calls GET on path and returns the raw response body.
func (c *Client) GetRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	c.addAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.addAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return c.readError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) addAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c *Client) readError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(b, &e); err == nil && e.Error != "" {
		return fmt.Errorf("%s (HTTP %d)", e.Error, resp.StatusCode)
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
}
