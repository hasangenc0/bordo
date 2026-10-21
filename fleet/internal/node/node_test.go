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
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered", "region": "us-east"})
	}))
	defer srv.Close()

	if err := register(context.Background(), srv.Client(), srv.URL, "us-east", "regtok", "KUBECONFIG"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got["region"] != "us-east" || got["register_token"] != "regtok" || got["kubeconfig"] != "KUBECONFIG" {
		t.Fatalf("unexpected register body: %+v", got)
	}
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
