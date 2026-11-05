package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hasangenc0/bordo/control-plane/internal/fleet"
)

// A reconciler reports its machine's disk on every pull; the fleet overview must carry the last
// reading, and explain an unreachable region by pointing at a full disk first.
func TestRegionDiskIsRecordedAndSurfaced(t *testing.T) {
	s, db := newStatusTestServer(t)
	db.Exec(`INSERT INTO regions (id, name, status) VALUES ('reg1','eu','pending')`)
	token, err := fleet.New(db).IssueClusterToken(context.Background(), "eu")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/fleet/desired-state", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Bordo-Disk-Total", "40800000000")
	req.Header.Set("X-Bordo-Disk-Free", "1500000000")
	s.handleFleetDesiredState(httptest.NewRecorder(), req)

	overview := func() map[string]any {
		w := httptest.NewRecorder()
		s.handleFleetOverview(w, httptest.NewRequest(http.MethodGet, "/v1/fleet", nil))
		var out struct {
			Regions []map[string]any `json:"regions"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Regions) != 1 {
			t.Fatalf("overview: %v %s", err, w.Body.String())
		}
		return out.Regions[0]
	}

	disk, _ := overview()["disk"].(map[string]any)
	if disk == nil || disk["level"] != "critical" || disk["free_bytes"].(float64) != 1.5e9 {
		t.Fatalf("disk not surfaced: %v", overview())
	}
	if _, has := overview()["hint"]; has {
		t.Fatalf("a region that just pulled needs no hint: %v", overview())
	}

	// Pulls go quiet: the region reads unreachable and the hint leads with the full disk.
	db.Exec(`UPDATE regions SET last_pull_at = datetime('now', '-10 minutes')`)
	region := overview()
	if region["status"] != "unreachable" {
		t.Fatalf("status = %v, want unreachable", region["status"])
	}
	if hint, _ := region["hint"].(string); !strings.Contains(hint, "disk was critical") || !strings.Contains(hint, "kubectl describe node") {
		t.Fatalf("hint should point at the disk: %q", hint)
	}

	// Garbage headers are ignored and never overwrite the last good reading.
	bad := httptest.NewRequest(http.MethodGet, "/fleet/desired-state", nil)
	bad.Header.Set("Authorization", "Bearer "+token)
	bad.Header.Set("X-Bordo-Disk-Total", "nope")
	bad.Header.Set("X-Bordo-Disk-Free", "5")
	s.handleFleetDesiredState(httptest.NewRecorder(), bad)
	if d, _ := overview()["disk"].(map[string]any); d == nil || d["free_bytes"].(float64) != 1.5e9 {
		t.Fatalf("bad headers changed the reading: %v", overview()["disk"])
	}
}
