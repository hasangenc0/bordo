package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const wl = `{"regions":[{"region":"eu","host_addr":"203.0.113.7","services":[{"node_ports":[30775]}]}]}`

func TestProbeTarget(t *testing.T) {
	got, err := probeTarget([]byte(wl), "eu", 0, "api/v1/events?limit=1")
	if err != nil || got != "http://203.0.113.7:30775/api/v1/events?limit=1" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := probeTarget([]byte(wl), "eu", 22, "/"); err == nil {
		t.Fatal("unreported port must be refused")
	}
	if _, err := probeTarget([]byte(wl), "us", 0, "/"); err == nil {
		t.Fatal("unknown region must error")
	}
	noHost := `{"regions":[{"region":"eu","services":[{"node_ports":[30775]}]}]}`
	if _, err := probeTarget([]byte(noHost), "eu", 0, "/"); err == nil || !strings.Contains(err.Error(), "region_set_host") {
		t.Fatalf("missing host err = %v", err)
	}
}

func TestProbeGetTruncates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("x", probeMaxBody+10)))
	}))
	defer srv.Close()
	out, err := probeGet(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["status"] != 200 || m["truncated"] != true || len(m["body"].(string)) != probeMaxBody {
		t.Fatalf("out = %+v", m)
	}
}
