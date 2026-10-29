package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ghcrBase is a variable so tests can point it at a fake registry.
var ghcrBase = "https://ghcr.io"

// VerifyGHCRPull asks ghcr.io itself whether this credential may pull repository ("owner/name"),
// which is the question that matters; a token's scope list is only a guess at the answer (GitHub
// may fold read:packages into write:packages when reporting scopes). It returns "" when the pull
// is allowed or the answer is unclear (network trouble, an unexpected reply), and otherwise a
// reason the cluster would hit ImagePullBackOff.
func VerifyGHCRPull(ctx context.Context, username, token, repository string) string {
	q := url.Values{"service": {"ghcr.io"}, "scope": {"repository:" + repository + ":pull"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ghcrBase+"/token?"+q.Encode(), nil)
	if err != nil {
		return ""
	}
	req.SetBasicAuth(username, token)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Sprintf("ghcr.io rejected the credential (HTTP %d): the token cannot read packages. Use a classic token with read:packages (bordo github connect)", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return ""
	}
	var body struct {
		Token string `json:"token"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if json.Unmarshal(raw, &body) != nil || body.Token == "" {
		return ""
	}
	// The registry token is a JWT whose "access" claim lists what was actually granted.
	parts := strings.Split(body.Token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var claims struct {
		Access []struct {
			Name    string   `json:"name"`
			Actions []string `json:"actions"`
		} `json:"access"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Access == nil {
		return ""
	}
	for _, a := range claims.Access {
		if strings.EqualFold(a.Name, repository) {
			for _, act := range a.Actions {
				if act == "pull" {
					return ""
				}
			}
		}
	}
	return fmt.Sprintf("ghcr.io accepts the credential but grants no pull access to %s: the token lacks read:packages, or the package is not visible to this account", repository)
}

// TokenInfo says what GitHub currently thinks of a stored token.
type TokenInfo struct {
	Hint   string `json:"hint"`   // first characters only, to compare with the token a cluster holds
	Valid  bool   `json:"valid"`  // GitHub accepts it
	Status int    `json:"status"` // HTTP status of the check (0: GitHub unreachable)
	Scopes string `json:"scopes,omitempty"`
	Login  string `json:"login,omitempty"`
}

// InspectToken asks GitHub whether token is still accepted and which scopes it carries. A token
// that has been revoked (GitHub keeps only a handful per user and OAuth app) answers 401.
func InspectToken(ctx context.Context, token string) TokenInfo {
	info := TokenInfo{}
	if len(token) >= 8 {
		info.Hint = token[:8]
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+"/user", nil)
	if err != nil {
		return info
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return info
	}
	defer resp.Body.Close()
	info.Status = resp.StatusCode
	info.Valid = resp.StatusCode == http.StatusOK
	info.Scopes = resp.Header.Get("X-OAuth-Scopes")
	var u struct {
		Login string `json:"login"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&u)
	info.Login = u.Login
	return info
}
