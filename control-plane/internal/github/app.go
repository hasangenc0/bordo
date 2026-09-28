package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

