package tools

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBlockedIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"127.0.0.1": true, "10.1.2.3": true, "172.18.0.2": true, "192.168.1.1": true,
		"169.254.169.254": true, "100.100.1.1": true, "0.0.0.0": true, "::1": true, "fd00::1": true,
		"8.8.8.8": false, "203.0.113.5": false, "2606:4700::1111": false,
	} {
		if got := blockedIP(net.ParseIP(ip)); got != want {
			t.Errorf("blockedIP(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestHTTPFetchRefusesLoopbackByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("secret")) }))
	defer srv.Close()
	if _, err := httpFetch(context.Background(), newFetchClient(), srv.URL, "", "", 100); err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("loopback fetch must be refused, got %v", err)
	}
}

func TestHTTPFetchReturnsStatusHeadersAndTruncatedBody(t *testing.T) {
	fetchAllowLoopback = true
	defer func() { fetchAllowLoopback = false }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(strings.Repeat("a", 50)))
	}))
	defer srv.Close()
	out, err := httpFetch(context.Background(), newFetchClient(), srv.URL+"/old", "GET", "Mozilla/5.0", 10)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["status"] != 405 || m["truncated"] != true || len(m["body"].(string)) != 10 || !strings.HasSuffix(m["final_url"].(string), "/new") {
		t.Fatalf("out = %+v", m)
	}
	if m["headers"].(map[string]string)["Content-Type"] != "text/html" {
		t.Fatalf("headers = %+v", m["headers"])
	}
}

func TestHTTPFetchValidatesInput(t *testing.T) {
	for _, u := range []string{"", "ftp://x.com", "file:///etc/passwd", "http://"} {
		if _, err := httpFetch(context.Background(), newFetchClient(), u, "", "", 10); err == nil {
			t.Errorf("url %q should be rejected", u)
		}
	}
	if _, err := httpFetch(context.Background(), newFetchClient(), "https://example.com", "POST", "", 10); err == nil {
		t.Error("POST must be rejected")
	}
}
