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
	ds, _, err := a.fetchDesiredState(context.Background())
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
	if _, _, err := a.fetchDesiredState(context.Background()); err == nil {
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

func TestReconcileReportsAppliedReleases(t *testing.T) {
	var reportBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fleet/desired-state":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"manifests":   "APPLY-ME",
				"release_ids": []string{"rel-1", "rel-2"},
			})
		case "/fleet/status":
			_ = json.NewDecoder(r.Body).Decode(&reportBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	a := New(Config{CPURL: srv.URL, ClusterToken: "ct", Interval: time.Second, Namespace: "bordo-apps"})
	a.apply = func(ctx context.Context, ns, m string) error { return nil }
	if err := a.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reportBody == nil {
		t.Fatal("reconciler did not report applied releases")
	}
	ids, _ := reportBody["applied_release_ids"].([]any)
	if len(ids) != 2 {
		t.Fatalf("applied_release_ids = %v", reportBody["applied_release_ids"])
	}
	if errStr, _ := reportBody["error"].(string); errStr != "" {
		t.Errorf("expected no error on successful apply, got %q", errStr)
	}
}

func TestReconcileReportsApplyFailure(t *testing.T) {
	var reportBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fleet/desired-state":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"manifests":   "APPLY-ME",
				"release_ids": []string{"rel-1"},
			})
		case "/fleet/status":
			_ = json.NewDecoder(r.Body).Decode(&reportBody)
		}
	}))
	defer srv.Close()

	a := New(Config{CPURL: srv.URL, ClusterToken: "ct", Interval: time.Second, Namespace: "bordo-apps"})
	a.apply = func(ctx context.Context, ns, m string) error { return context.DeadlineExceeded }
	if err := a.ReconcileOnce(context.Background()); err == nil {
		t.Fatal("expected apply error to propagate")
	}
	if reportBody == nil {
		t.Fatal("reconciler did not report the failed apply")
	}
	if errStr, _ := reportBody["error"].(string); errStr == "" {
		t.Error("expected the apply error to be reported")
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

func TestParseObservation(t *testing.T) {
	raw := []byte(`{"items":[
	 {"kind":"Service","metadata":{"name":"api"},"spec":{"type":"NodePort","ports":[{"port":8080,"nodePort":30123}]}},
	 {"kind":"Pod","metadata":{"name":"api-1"},"status":{"phase":"Pending","containerStatuses":[{"ready":false,"state":{"waiting":{"reason":"ImagePullBackOff"}}}]}},
	 {"kind":"Pod","metadata":{"name":"api-2"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"state":{}}]}}]}`)
	obs, err := parseObservation(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Services) != 1 || obs.Services[0].NodePorts[0] != 30123 {
		t.Fatalf("services = %+v", obs.Services)
	}
	if len(obs.Pods) != 2 || obs.Pods[0].Reason != "ImagePullBackOff" || obs.Pods[0].Ready || !obs.Pods[1].Ready {
		t.Fatalf("pods = %+v", obs.Pods)
	}
}

func TestReconcileOnceReportsObservation(t *testing.T) {
	var got struct {
		IDs      []string     `json:"applied_release_ids"`
		Observed *Observation `json:"observed"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fleet/status" {
			_ = json.NewDecoder(r.Body).Decode(&got)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"manifests": "x", "release_ids": []string{"r1"}})
	}))
	defer srv.Close()
	a := New(Config{CPURL: srv.URL, ClusterToken: "ct", Interval: time.Second, Namespace: "ns"})
	a.apply = func(context.Context, string, string) error { return nil }
	a.observe = func(context.Context, string) (*Observation, error) {
		return &Observation{Services: []ServiceInfo{{Name: "api", NodePorts: []int{30001}}}}, nil
	}
	if err := a.ReconcileOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got.IDs) != 1 || got.Observed == nil || got.Observed.Services[0].NodePorts[0] != 30001 {
		t.Fatalf("report = %+v", got)
	}
}
