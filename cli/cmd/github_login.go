package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hasangenc0/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// defaultGitHubClientID is the public OAuth App client id baked into the binary.
// It is safe to be public (device flow is a public-client flow, no secret).
// Register a public OAuth App with "Enable Device Flow" on and set this, or
// override at runtime with BORDO_GITHUB_CLIENT_ID / --client-id.
const defaultGitHubClientID = ""

// githubScopes are the OAuth scopes Bordo needs: create/push repos, manage the
// build workflow, and push/pull GHCR images.
const githubScopes = "repo workflow write:packages"

func githubLoginCmd() *cobra.Command {
	var clientID string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Connect GitHub via device flow (no browser on the server)",
		Long: `Connect your GitHub account using GitHub's device flow.

Prints a short code to enter at https://github.com/login/device from any
browser (your laptop or phone) — nothing opens on the server, no tunnel or
base_url needed. Bordo then uses your token to create repos, run builds in
GitHub Actions, and pull private images.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cid := clientID
			if cid == "" {
				cid = os.Getenv("BORDO_GITHUB_CLIENT_ID")
			}
			if cid == "" {
				cid = defaultGitHubClientID
			}
			if cid == "" {
				return fmt.Errorf("no GitHub OAuth App client id configured.\n" +
					"Register a public OAuth App (GitHub > Settings > Developer settings > OAuth Apps)\n" +
					"with \"Enable Device Flow\" turned on, then set its client id:\n" +
					"  export BORDO_GITHUB_CLIENT_ID=<client_id>\n" +
					"or pass --client-id. Alternatively paste a token directly: bordo github connect")
			}

			da, err := startDeviceFlow(cid, githubScopes)
			if err != nil {
				return err
			}

			fmt.Printf("\nConnect GitHub:\n  1. Open %s\n  2. Enter the code:  %s\n\nWaiting for authorization (Ctrl-C to cancel)...\n",
				da.VerificationURI, da.UserCode)

			expiry := da.ExpiresIn
			if expiry <= 0 {
				expiry = 900
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(expiry)*time.Second)
			defer cancel()

			token, err := pollDeviceToken(ctx, cid, da.DeviceCode, da.Interval)
			if err != nil {
				return err
			}
			if err := storeGithubToken(token); err != nil {
				return fmt.Errorf("saving token to server: %w", err)
			}
			fmt.Println("\n✓ GitHub connected. Builds will now run in GitHub Actions.")
			return nil
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "GitHub OAuth App client id (or set BORDO_GITHUB_CLIENT_ID)")
	return cmd
}

// githubConnectCmd is the fallback: paste a Personal Access Token directly.
func githubConnectCmd() *cobra.Command {
	var token string
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect GitHub with a Personal Access Token (fallback for 'login')",
		Long: `Store a GitHub token on the Bordo server directly.

Create a fine-grained PAT at github.com with Contents, Actions, and Packages
read/write on the repos you want Bordo to manage, then run this with --token
(or paste it when prompted).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			t := strings.TrimSpace(token)
			if t == "" {
				fmt.Print("GitHub token: ")
				_, _ = fmt.Scanln(&t)
				t = strings.TrimSpace(t)
			}
			if t == "" {
				return fmt.Errorf("a token is required")
			}
			if err := storeGithubToken(t); err != nil {
				return err
			}
			fmt.Println("✓ GitHub connected.")
			return nil
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "GitHub Personal Access Token")
	return cmd
}

type deviceAuth struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func startDeviceFlow(clientID, scope string) (*deviceAuth, error) {
	form := url.Values{"client_id": {clientID}, "scope": {scope}}
	req, _ := http.NewRequest(http.MethodPost, "https://github.com/login/device/code", strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var da deviceAuth
	if err := json.NewDecoder(resp.Body).Decode(&da); err != nil {
		return nil, err
	}
	if da.DeviceCode == "" {
		return nil, fmt.Errorf("GitHub did not start the device flow — check the client id and that \"Enable Device Flow\" is on for the OAuth App")
	}
	return &da, nil
}

func pollDeviceToken(ctx context.Context, clientID, deviceCode string, interval int) (string, error) {
	if interval <= 0 {
		interval = 5
	}
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out waiting for authorization — run 'bordo github login' again")
		case <-time.After(time.Duration(interval) * time.Second):
		}
		form := url.Values{
			"client_id":   {clientID},
			"device_code": {deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}
		req, _ := http.NewRequest(http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		var r struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
			ErrorDesc   string `json:"error_description"`
			Interval    int    `json:"interval"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&r)
		_ = resp.Body.Close()
		if r.AccessToken != "" {
			return r.AccessToken, nil
		}
		switch r.Error {
		case "authorization_pending", "":
			// keep polling
		case "slow_down":
			if r.Interval > 0 {
				interval = r.Interval
			} else {
				interval += 5
			}
		case "expired_token":
			return "", fmt.Errorf("the code expired — run 'bordo github login' again")
		case "access_denied":
			return "", fmt.Errorf("authorization was denied")
		default:
			return "", fmt.Errorf("device flow error: %s — %s", r.Error, r.ErrorDesc)
		}
	}
}

// storeGithubToken sends the obtained token to the control plane to persist.
func storeGithubToken(token string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Server == "" {
		return fmt.Errorf("not configured — run 'bordo config set server <url>' first")
	}
	body, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequest(http.MethodPost, cfg.Server+"/v1/github/user-token", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, e["error"])
	}
	return nil
}
