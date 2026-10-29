package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/hasangenc0/bordo/control-plane/internal/fleet"
)

func pullDesired(s *Server, token, revision string) map[string]any {
	req := httptest.NewRequest(http.MethodGet, "/fleet/desired-state", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if revision != "" {
		req.Header.Set("X-Bordo-Reconciler-Revision", revision)
	}
	w := httptest.NewRecorder()
	s.handleFleetDesiredState(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

func requestUpgrade(s *Server, region, query string) (int, map[string]any) {
	req := httptest.NewRequest(http.MethodPost, "/v1/fleet/regions/"+region+"/reconciler/upgrade"+query, nil)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("name", region)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	s.handleReconcilerUpgrade(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestReconcilerUpgradeFlow(t *testing.T) {
	s, db := newStatusTestServer(t)
	db.Exec(`INSERT INTO regions (id, name, status) VALUES ('reg1','eu','pending')`)
	token, err := fleet.New(db).IssueClusterToken(context.Background(), "eu")
	if err != nil {
		t.Fatal(err)
	}

	// An unknown region is a 404.
	if code, _ := requestUpgrade(s, "nope", ""); code != http.StatusNotFound {
		t.Fatalf("unknown region: code %d", code)
	}

	// A reconciler that reports no revision predates self-restart: it cannot be asked.
	pullDesired(s, token, "")
	_, out := requestUpgrade(s, "eu", "")
	if out["restart_requested"] != false {
		t.Fatalf("an old reconciler must not be asked to restart: %v", out)
	}
	if info := out["reconciler"].(map[string]any); info["can_self_upgrade"] != false || info["current"] != false {
		t.Fatalf("old reconciler state wrong: %v", info)
	}

	// A current reconciler is left alone unless forced.
	pullDesired(s, token, "1")
	if _, out = requestUpgrade(s, "eu", ""); out["restart_requested"] != false {
		t.Fatalf("a current reconciler must not be restarted: %v", out)
	}

	// A forced restart is delivered once; the ask is cleared after delivery.
	s.recordReconcilerRevision("eu", 1)
	if _, out = requestUpgrade(s, "eu", "?force=true"); out["restart_requested"] != true {
		t.Fatalf("force must request a restart: %v", out)
	}
	if got := pullDesired(s, token, "1"); got["restart"] != true {
		t.Fatalf("next pull must carry restart=true, got %v", got)
	}
	if got := pullDesired(s, token, "1"); got["restart"] == true {
		t.Fatalf("restart must be delivered only once, got %v", got)
	}
}

func TestOutdatedReconcilerIsFlaggedInTheReleaseLog(t *testing.T) {
	rep := workloadReport{Pods: []observedPod{{Name: "p", Phase: "Running", Ready: true}}}
	info := reconcilerState(0)
	rep.Reconciler = &info
	if got := describeWorkloads(rep); !strings.Contains(got, "bordo region upgrade") {
		t.Errorf("outdated reconciler not flagged: %q", got)
	}
	info = reconcilerState(fleet.ExpectedReconcilerRevision)
	if got := describeWorkloads(rep); strings.Contains(got, "outdated") {
		t.Errorf("current reconciler flagged as outdated: %q", got)
	}
}

func TestFirstGHCRRepo(t *testing.T) {
	for in, want := range map[string]string{
		"image: ghcr.io/Owner/City-Events-API:abc123\n": "owner/city-events-api",
		`image: "ghcr.io/o/app@sha256:xx"`:              "o/app",
		"image: nginx":                                  "",
	} {
		if got := firstGHCRRepo(in); got != want {
			t.Errorf("firstGHCRRepo(%q) = %q, want %q", in, got, want)
		}
	}
}
