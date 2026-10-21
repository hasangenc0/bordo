package node

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("BORDO_CP_URL", "https://cp.example.com/")
	t.Setenv("BORDO_JOIN_TOKEN", "id.secret")
	t.Setenv("BORDO_NODE_NAME", "node-1")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.CPURL != "https://cp.example.com" { // trailing slash trimmed
		t.Fatalf("CPURL = %q", cfg.CPURL)
	}
	if cfg.K3sVersion != DefaultK3sVersion {
		t.Fatalf("K3sVersion = %q, want default", cfg.K3sVersion)
	}
	if cfg.NodeName != "node-1" {
		t.Fatalf("NodeName = %q", cfg.NodeName)
	}
}

func TestConfigFromEnvRequiresFields(t *testing.T) {
	os.Unsetenv("BORDO_CP_URL")
	os.Unsetenv("BORDO_JOIN_TOKEN")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected error when required env vars are missing")
	}
}

func TestRedeem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fleet/redeem" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["token"] != "jointok" {
			t.Errorf("token = %q", body["token"])
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"region": "us-east", "register_token": "regtok"})
	}))
	defer srv.Close()

	rr, err := redeem(context.Background(), srv.Client(), srv.URL, "jointok")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if rr.Region != "us-east" || rr.RegisterToken != "regtok" {
		t.Fatalf("unexpected: %+v", rr)
	}
}

func TestRedeemRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired join token"})
	}))
	defer srv.Close()

	if _, err := redeem(context.Background(), srv.Client(), srv.URL, "bad"); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestRedeemIncompleteResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"region": "us-east"}) // missing register_token
	}))
	defer srv.Close()

	if _, err := redeem(context.Background(), srv.Client(), srv.URL, "x"); err == nil {
		t.Fatal("expected error when register_token is missing")
	}
}

func TestRegister(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fleet/register" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered", "region": "us-east", "cluster_token": "CT"})
	}))
	defer srv.Close()

	ct, err := register(context.Background(), srv.Client(), srv.URL, "us-east", "regtok", "KUBECONFIG")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if ct != "CT" {
		t.Fatalf("cluster token = %q, want CT", ct)
	}
	if got["region"] != "us-east" || got["register_token"] != "regtok" || got["kubeconfig"] != "KUBECONFIG" {
		t.Fatalf("unexpected register body: %+v", got)
	}
}

func TestRenderReconcilerManifests(t *testing.T) {
	m := renderReconcilerManifests(reconcilerVars{
		CPURL:        "https://cp:7401",
		ClusterToken: "CT-SECRET",
		Image:        "ghcr.io/x/bordo-reconciler:latest",
		Namespace:    "bordo-apps",
	})
	for _, want := range []string{
		"kind: Namespace",
		"name: bordo-system",
		"kind: Role",            // namespace-scoped RBAC, not ClusterRole
		"namespace: bordo-apps", // Role scoped to the apps namespace
		"serviceAccountName: bordo-reconciler",
		"cluster-token: CT-SECRET",
		"image: ghcr.io/x/bordo-reconciler:latest",
		`value: "https://cp:7401"`,
	} {
		if !contains(m, want) {
			t.Errorf("rendered manifests missing %q", want)
		}
	}
	// It must NOT grant cluster-admin / ClusterRole.
	if contains(m, "ClusterRole") {
		t.Error("reconciler must use a namespace Role, not a ClusterRole")
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	f := t.TempDir() + "/data"
	if err := os.WriteFile(f, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	// echo -n hello | sha256sum
	const helloSHA = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if err := verifyFileSHA256(f, helloSHA); err != nil {
		t.Fatalf("matching sha should pass: %v", err)
	}
	if err := verifyFileSHA256(f, "deadbeef"); err == nil {
		t.Fatal("mismatched sha should fail")
	}
}

func TestDownloadFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("script-body"))
	}))
	defer srv.Close()

	dest := t.TempDir() + "/dl"
	if err := downloadFile(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "script-body" {
		t.Fatalf("downloaded %q", string(b))
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestReadKubeconfigPatchesAddress(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "kc")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("server: https://127.0.0.1:6443\n")
	f.Close()

	kc, err := readKubeconfig(f.Name(), "5.75.1.2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "server: https://5.75.1.2:6443\n"; kc != want {
		t.Fatalf("kubeconfig = %q, want %q", kc, want)
	}
}

// TestRunSkipK3s exercises the full orchestration without touching the host:
// with SkipK3s set, Run redeems and returns before installing k3s or registering.
func TestRunSkipK3s(t *testing.T) {
	var redeemCalled, registerCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fleet/redeem":
			redeemCalled = true
			_ = json.NewEncoder(w).Encode(map[string]string{"region": "r", "register_token": "rt"})
		case "/fleet/register":
			registerCalled = true
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := Run(context.Background(), Config{CPURL: srv.URL, JoinToken: "jt", SkipK3s: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !redeemCalled {
		t.Fatal("expected redeem to be called")
	}
	if registerCalled {
		t.Fatal("register should not be called in skip mode")
	}
}
