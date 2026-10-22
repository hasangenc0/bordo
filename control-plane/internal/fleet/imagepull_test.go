package fleet

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderImagePullSecret(t *testing.T) {
	m, err := RenderImagePullSecret("bordo-apps", "ghcr.io", "alice", "tok123")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"kind: Secret",
		"name: bordo-registry-creds",
		"namespace: bordo-apps",
		"type: kubernetes.io/dockerconfigjson",
		".dockerconfigjson:",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest missing %q", want)
		}
	}

	// The embedded dockerconfigjson must decode and contain the registry auth.
	var line string
	for _, l := range strings.Split(m, "\n") {
		if strings.Contains(l, ".dockerconfigjson:") {
			line = strings.TrimSpace(strings.SplitN(l, ":", 2)[1])
		}
	}
	raw, err := base64.StdEncoding.DecodeString(line)
	if err != nil {
		t.Fatalf("decode dockerconfigjson: %v", err)
	}
	var cfg struct {
		Auths map[string]struct {
			Username, Password, Auth string
		} `json:"auths"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	a, ok := cfg.Auths["ghcr.io"]
	if !ok {
		t.Fatal("missing ghcr.io auth entry")
	}
	if a.Username != "alice" || a.Password != "tok123" {
		t.Fatalf("bad creds: %+v", a)
	}
	wantAuth := base64.StdEncoding.EncodeToString([]byte("alice:tok123"))
	if a.Auth != wantAuth {
		t.Fatalf("auth = %q, want %q", a.Auth, wantAuth)
	}
}
