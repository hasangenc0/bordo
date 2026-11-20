package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func searchServer(t *testing.T, status int, body string, got *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/search" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("unexpected request %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		if got != nil {
			_ = json.NewDecoder(r.Body).Decode(got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestWebSearchMapsResults(t *testing.T) {
	var sent map[string]any
	srv := searchServer(t, 200, `{"results":[
		{"title":"A","url":"https://a.dev","snippet":"snip","summary":"long summary","content":"`+strings.Repeat("x", 5000)+`","site_name":"A Site","date":"2026-06-01T00:00:00+00:00","authority":3},
		{"title":"B","url":"https://b.dev","snippet":"only snippet","summary":"","content":"","site_name":null,"date":null,"authority":0}]}`, &sent)
	defer srv.Close()

	out, err := webSearch(context.Background(), srv.Client(), srv.URL, "sk-test", "  go 1.25  ", 99, true)
	if err != nil {
		t.Fatal(err)
	}
	if sent["query"] != "go 1.25" || sent["count"] != float64(20) || sent["need_content"] != true {
		t.Fatalf("sent = %+v", sent)
	}
	rs := out.(map[string]any)["results"].([]map[string]any)
	if len(rs) != 2 || rs[0]["summary"] != "long summary" || rs[0]["authoritative"] != true || rs[0]["site"] != "A Site" {
		t.Fatalf("results = %+v", rs)
	}
	if n := len([]rune(rs[0]["content"].(string))); n != searchContentChars+1 {
		t.Fatalf("content length %d, want truncated to %d+ellipsis", n, searchContentChars)
	}
	if rs[1]["summary"] != "only snippet" || rs[1]["date"] != nil || rs[1]["authoritative"] != nil {
		t.Fatalf("second = %+v", rs[1])
	}
}

func TestWebSearchEmptyResultsHasNote(t *testing.T) {
	srv := searchServer(t, 200, `{"results":[]}`, nil)
	defer srv.Close()
	out, err := webSearch(context.Background(), srv.Client(), srv.URL, "sk-test", "zzzz", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if out.(map[string]any)["note"] == nil {
		t.Fatalf("out = %+v", out)
	}
}

func TestWebSearchValidatesBeforeCalling(t *testing.T) {
	c := http.DefaultClient
	if _, err := webSearch(context.Background(), c, "http://invalid.invalid", "sk", "   ", 5, false); err == nil {
		t.Error("blank query must fail")
	}
	if _, err := webSearch(context.Background(), c, "http://invalid.invalid", "sk", strings.Repeat("é", 129), 5, false); err == nil {
		t.Error("129-char query must fail")
	}
	if _, err := webSearch(context.Background(), c, "http://invalid.invalid", "", "ok", 5, false); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("missing key: %v", err)
	}
	if _, err := webSearch(context.Background(), c, "http://invalid.invalid", "sk", strings.Repeat("é", 128), 5, false); err == nil || strings.Contains(err.Error(), "too long") {
		t.Errorf("128 runes is valid and should reach the network: %v", err)
	}
}

func TestWebSearchErrorStatuses(t *testing.T) {
	for status, want := range map[int]string{401: "key was rejected", 402: "insufficient balance", 429: "rate limited", 400: "count must", 504: "HTTP 504"} {
		srv := searchServer(t, status, `{"error":{"message":"count must be 1-50"}}`, nil)
		_, err := webSearch(context.Background(), srv.Client(), srv.URL, "sk-test", "q", 5, false)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("status %d: err = %v, want %q", status, err, want)
		}
	}
}

func TestWebSearchToolReadsKeyPerCall(t *testing.T) {
	srv := searchServer(t, 200, `{"results":[]}`, nil)
	defer srv.Close()
	key := ""
	tool := WebSearchTool(srv.URL, func() string { return key })
	if _, err := tool.Call(context.Background(), map[string]any{"query": "q"}); err == nil {
		t.Fatal("expected not-configured error")
	}
	key = "sk-test"
	if _, err := tool.Call(context.Background(), map[string]any{"query": "q"}); err != nil {
		t.Fatalf("key set later must work: %v", err)
	}
}
