package cfaccess

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// roundTrip records the headers each request carried instead of touching the network.
type recorder struct{ last http.Header }

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.last = req.Header.Clone()
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: req}, nil
}

func newTest(t *testing.T) (*transport, *recorder) {
	t.Helper()
	mu.Lock()
	hosts = map[string]creds{}
	mu.Unlock()
	rec := &recorder{}
	return &transport{next: rec}, rec
}

func register(t *testing.T, server, id, secret string) {
	t.Helper()
	u, _ := url.Parse(server)
	if id != "" && secret != "" && u.Scheme == "https" {
		mu.Lock()
		hosts[hostKey(u)] = creds{id, secret}
		mu.Unlock()
	}
}

func do(t *testing.T, tr *transport, rawURL string) (*http.Request, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, rawURL, nil)
	req.RequestURI = ""
	_, err := tr.RoundTrip(req)
	return req, err
}

func TestHeadersOnlyToRegisteredHTTPSHost(t *testing.T) {
	tr, rec := newTest(t)
	register(t, "https://api.example.com", "cid", "csecret")

	req, err := do(t, tr, "https://api.example.com/v1/projects")
	if err != nil {
		t.Fatal(err)
	}
	if rec.last.Get(headerID) != "cid" || rec.last.Get(headerSecret) != "csecret" {
		t.Errorf("registered host got headers %v", rec.last)
	}
	if req.Header.Get(headerID) != "" {
		t.Error("the caller's request was modified")
	}

	for _, other := range []string{
		"https://github.com/login/device/code",    // another host
		"https://api.example.com:8443/v1",         // same host, other port
		"http://api.example.com/v1",               // plaintext
		"https://api.example.com.evil.test/steal", // look-alike
	} {
		rec.last = nil
		if _, err := do(t, tr, other); err != nil {
			t.Fatal(err)
		}
		if rec.last.Get(headerID) != "" || rec.last.Get(headerSecret) != "" {
			t.Errorf("%s received the service token", other)
		}
	}
}

func TestInstallIgnoresIncompleteOrPlainHTTP(t *testing.T) {
	newTest(t)
	for _, c := range []struct{ server, id, secret string }{
		{"https://api.example.com", "", "s"},
		{"https://api.example.com", "i", ""},
		{"http://api.example.com", "i", "s"},
		{"", "i", "s"},
		{"not a url", "i", "s"},
	} {
		Install(c.server, c.id, c.secret)
	}
	mu.RLock()
	defer mu.RUnlock()
	if len(hosts) != 0 {
		t.Errorf("registered %v, want nothing", hosts)
	}
}
