package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func regionAddAgainst(t *testing.T, register int) (map[string]any, error) {
	t.Helper()
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/regions":
			w.WriteHeader(register)
			_, _ = w.Write([]byte(`{"error":"region \"eu-central\" already exists"}`))
		case r.URL.Path == "/v1/regions/eu-central/join-tokens":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"fresh-token","expires_at":"2026-10-23T12:00:00Z"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(cp.Close)

	out, err := regionAddTool(NewCPClient(cp.URL, "t")).Call(context.Background(), map[string]any{"name": "eu-central"})
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

func TestRegionAddMintsFreshTokenForExistingRegion(t *testing.T) {
	out, err := regionAddAgainst(t, http.StatusConflict)
	if err != nil {
		t.Fatalf("existing region must not be an error: %v", err)
	}
	if out["already_registered"] != true {
		t.Errorf("already_registered = %v, want true", out["already_registered"])
	}
	if cmd, _ := out["join_command"].(string); !strings.Contains(cmd, "BORDO_JOIN_TOKEN=fresh-token") {
		t.Errorf("join command lacks the fresh token: %q", cmd)
	}
}

func TestRegionAddStillFailsOnOtherRegistrationErrors(t *testing.T) {
	if _, err := regionAddAgainst(t, http.StatusInternalServerError); err == nil {
		t.Fatal("a 500 from register must still fail")
	}
}
