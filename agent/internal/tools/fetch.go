package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	fetchDefaultBytes = 20 << 10
	fetchMaxBytes     = 200 << 10
	fetchMaxRedirects = 5
)

// fetchAllowLoopback lets tests reach httptest servers; never set in production.
var fetchAllowLoopback = false

// blockedIP reports addresses http_fetch must never connect to: this machine, the Docker or
// LAN networks (including the control plane and other services), link-local (cloud metadata
// at 169.254.169.254) and CGNAT/tailnet ranges.
func blockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() {
		return !fetchAllowLoopback
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xC0 == 64 { // 100.64.0.0/10
		return true
	}
	return false
}

// newFetchClient builds an HTTP client that checks the address actually dialled (after DNS
// resolution, on every redirect hop), so neither a hostname that resolves to a private
// address nor a DNS rebind can reach internal services.
func newFetchClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if blockedIP(net.ParseIP(host)) {
				return fmt.Errorf("refusing to connect to non-public address %s", host)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext, ResponseHeaderTimeout: 15 * time.Second},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= fetchMaxRedirects {
				return fmt.Errorf("stopped after %d redirects", fetchMaxRedirects)
			}
			return nil
		},
	}
}

// httpFetchTool is the agent's curl: a read-only request to a public web page or API, for
// checking what a site really returns (status, headers, markup) before writing code against it.
func httpFetchTool() *Tool {
	return &Tool{
		Name: "http_fetch",
		Description: "Fetch a public http(s) URL (GET or HEAD) and return the status, final URL, key response headers and the start of the body, like `curl -sIL` / `curl -sL | head`. " +
			"Use it to see what a website actually returns before writing a scraper or parser, instead of asking the user to run curl. " +
			"Public internet only: private, loopback and cloud-metadata addresses are refused, so it cannot reach Bordo's own services or the user's apps (use app_probe for those). Read-only.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":        map[string]any{"type": "string", "description": "http:// or https:// URL"},
				"method":     map[string]any{"type": "string", "enum": []string{"GET", "HEAD"}, "description": "Default GET"},
				"max_bytes":  map[string]any{"type": "integer", "description": "Body bytes to return (default 20000, max 200000)"},
				"user_agent": map[string]any{"type": "string", "description": "Override the User-Agent, e.g. a browser string to see what a browser would get"},
			},
			"required": []string{"url"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			raw, _ := params["url"].(string)
			method, _ := params["method"].(string)
			ua, _ := params["user_agent"].(string)
			max := fetchDefaultBytes
			if f, ok := params["max_bytes"].(float64); ok && f > 0 {
				max = int(f)
			}
			return httpFetch(ctx, newFetchClient(), raw, method, ua, max)
		},
	}
}

func httpFetch(ctx context.Context, c *http.Client, raw, method, ua string, max int) (any, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("url must be an absolute http:// or https:// URL")
	}
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return nil, fmt.Errorf("only GET and HEAD are allowed")
	}
	if max > fetchMaxBytes {
		max = fetchMaxBytes
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if ua == "" {
		ua = "bordo-agent/1.0 (+https://github.com/hasangenc0/bordo)"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")

	start := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", method, u, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(max)+1))
	truncated := len(body) > max
	if truncated {
		body = body[:max]
	}
	text := strings.ToValidUTF8(string(body), "�")
	if !utf8.ValidString(text) {
		text = ""
	}
	headers := map[string]string{}
	for _, k := range []string{"Content-Type", "Content-Length", "Server", "Location", "Retry-After", "Cache-Control", "Content-Encoding", "X-Robots-Tag", "CF-Ray", "Via"} {
		if v := resp.Header.Get(k); v != "" {
			headers[k] = v
		}
	}
	return map[string]any{
		"url":        u.String(),
		"final_url":  resp.Request.URL.String(),
		"status":     resp.StatusCode,
		"headers":    headers,
		"elapsed_ms": time.Since(start).Milliseconds(),
		"body":       text,
		"truncated":  truncated,
	}, nil
}
