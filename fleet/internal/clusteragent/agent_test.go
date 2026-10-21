package clusteragent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("BORDO_CP_URL", "https://cp/")
	t.Setenv("BORDO_CLUSTER_TOKEN", "ct")
	t.Setenv("BORDO_RECONCILE_INTERVAL", "10")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CPURL != "https://cp" || cfg.ClusterToken != "ct" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Interval != 10*time.Second {
		t.Fatalf("interval = %s", cfg.Interval)
	}
	if cfg.Namespace != defaultNamespace {
		t.Fatalf("namespace = %q", cfg.Namespace)
	}
}

func TestFetchDesiredStateSendsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ct" {
			t.Errorf("auth header = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"region": "us-east", "manifests": "KIND: Deployment"})
	}))
	defer srv.Close()

	a := New(Config{CPURL: srv.URL, ClusterToken: "ct", Interval: time.Second, Namespace: "bordo-apps"})
	ds, err := a.fetchDesiredState(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if ds != "KIND: Deployment" {
		t.Fatalf("manifests = %q", ds)
	}
}

func TestFetchDesiredStateRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid cluster token"})
	}))
	defer srv.Close()

	a := New(Config{CPURL: srv.URL, ClusterToken: "bad", Interval: time.Second})
	if _, err := a.fetchDesiredState(context.Background()); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestReconcileOnceAppliesManifests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"manifests": "APPLY-ME"})
	}))
	defer srv.Close()

	var appliedNS, appliedManifests string
	a := New(Config{CPURL: srv.URL, ClusterToken: "ct", Interval: time.Second, Namespace: "bordo-apps"})
	a.apply = func(ctx context.Context, ns, m string) error {
		appliedNS, appliedManifests = ns, m
		return nil
	}
	if err := a.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if appliedNS != "bordo-apps" || appliedManifests != "APPLY-ME" {
		t.Fatalf("applied ns=%q manifests=%q", appliedNS, appliedManifests)
	}
}

func TestReconcileOnceEmptyIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"manifests": "   "})
	}))
	defer srv.Close()

	applied := false
	a := New(Config{CPURL: srv.URL, ClusterToken: "ct", Interval: time.Second})
	a.apply = func(ctx context.Context, ns, m string) error { applied = true; return nil }
	if err := a.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if applied {
		t.Fatal("empty desired state should not trigger apply")
	}
}
