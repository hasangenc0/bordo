// Package cfaccess lets Bordo clients reach a control plane that sits behind Cloudflare Access.
//
// Access identifies machines by a service token sent as two headers. Install registers the token
// for one server and wraps http.DefaultTransport once, so every http.Client that uses the default
// transport (http.DefaultClient, &http.Client{} without a Transport) carries it, without touching
// each call site. The headers go only to the registered host and only over https, so a redirect, a
// GitHub call or a plaintext URL never receives the secret.
package cfaccess

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	// Env names read by FromEnv. They are the same in every Bordo binary.
	EnvClientID     = "BORDO_CF_ACCESS_CLIENT_ID"
	EnvClientSecret = "BORDO_CF_ACCESS_CLIENT_SECRET"

	headerID     = "CF-Access-Client-Id"
	headerSecret = "CF-Access-Client-Secret"
)

type creds struct{ id, secret string }

var (
	mu    sync.RWMutex
	hosts = map[string]creds{} // lower-case host:port -> service token
	wrap  sync.Once
)

// FromEnv installs the service token from the environment for serverURL.
func FromEnv(serverURL string) {
	Install(serverURL, os.Getenv(EnvClientID), os.Getenv(EnvClientSecret))
}

// Install sends the service token to serverURL's host. It does nothing unless both values are set
// and serverURL is an https URL.
func Install(serverURL, id, secret string) {
	id, secret = strings.TrimSpace(id), strings.TrimSpace(secret)
	if id == "" || secret == "" {
		return
	}
	u, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return
	}
	mu.Lock()
	hosts[hostKey(u)] = creds{id, secret}
	mu.Unlock()
	wrap.Do(func() { http.DefaultTransport = &transport{next: http.DefaultTransport} })
}

func hostKey(u *url.URL) string {
	h := strings.ToLower(u.Hostname())
	p := u.Port()
	if p == "" {
		p = "443"
	}
	return h + ":" + p
}

type transport struct{ next http.RoundTripper }

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" {
		mu.RLock()
		c, ok := hosts[hostKey(req.URL)]
		mu.RUnlock()
		if ok {
			// A RoundTripper must not modify the caller's request.
			req = req.Clone(req.Context())
			req.Header.Set(headerID, c.id)
			req.Header.Set(headerSecret, c.secret)
		}
	}
	return t.next.RoundTrip(req)
}
