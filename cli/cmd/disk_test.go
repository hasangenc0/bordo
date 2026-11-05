package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hasangenc0/bordo/cli/internal/client"
)

func TestDiskReadingLevels(t *testing.T) {
	const gb = 1 << 30
	for _, c := range []struct {
		total, free uint64
		want        string
	}{
		{100 * gb, 60 * gb, "ok"},
		{100 * gb, 20 * gb, "warn"},
		{100 * gb, 10 * gb, "critical"},
		{38 * gb, 3 * gb / 2, "critical"},
	} {
		if got := (diskReading{Total: c.total, Free: c.free}).level(); got != c.want {
			t.Errorf("level(%d free of %d) = %s, want %s", c.free, c.total, got, c.want)
		}
	}
}

func TestReportDiskOnlyAdvisesWhenFilling(t *testing.T) {
	const gb = 1 << 30
	var ok, full bytes.Buffer
	reportDisk(&ok, diskReading{Total: 100 * gb, Free: 70 * gb, Path: "/var/lib/docker"})
	if strings.Contains(ok.String(), "docker builder prune") {
		t.Errorf("a healthy disk should get no advice: %q", ok.String())
	}
	reportDisk(&full, diskReading{Total: 38 * gb, Free: gb, Path: "/var/lib/docker"})
	for _, want := range []string{"97% used", "disk-pressure", "docker system df -v", "sandbox-cache"} {
		if !strings.Contains(full.String(), want) {
			t.Errorf("full-disk report missing %q: %q", want, full.String())
		}
	}
}

func TestPrintFleet(t *testing.T) {
	f := &client.Fleet{
		Regions: []client.FleetRegion{
			{Name: "eu", Status: "unreachable", Hint: "The reconciler has stopped pulling.",
				Disk: &client.DiskUsage{UsedPercent: 96, FreeBytes: 1_500_000_000, Level: "critical"}},
			{Name: "us", Status: "healthy", Disk: &client.DiskUsage{UsedPercent: 20, FreeBytes: 30 << 30, Level: "ok"}},
			{Name: "old", Status: "healthy"},
		},
		PlatformDisk: &client.DiskUsage{UsedPercent: 18, FreeBytes: 30 << 30, Level: "ok"},
	}
	var b bytes.Buffer
	printFleet(&b, f)
	out := b.String()
	for _, want := range []string{"96% used", "[CRITICAL]", "eu: The reconciler has stopped pulling.", "unknown (reconciler has not reported", "Bordo server disk: 18% used"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "us: ") {
		t.Errorf("a healthy region should have no explanation line:\n%s", out)
	}
}

// The patch printed for existing regions must be valid JSON, or kubectl rejects it.
func TestHardenReconcilerPatchIsValidJSON(t *testing.T) {
	i, j := strings.Index(hardenReconcilerCmd, "'"), strings.LastIndex(hardenReconcilerCmd, "'")
	var v map[string]any
	if err := json.Unmarshal([]byte(hardenReconcilerCmd[i+1:j]), &v); err != nil {
		t.Fatalf("patch is not JSON: %v", err)
	}
}
