package fleet

import (
	"context"
	"strings"
	"testing"
)

func TestNewDiskUsageLevels(t *testing.T) {
	const gb = int64(1 << 30)
	cases := []struct {
		total, free int64
		level       string
		pct         float64
	}{
		{100 * gb, 60 * gb, "ok", 40},
		{100 * gb, 20 * gb, "warn", 80},
		{100 * gb, 10 * gb, "critical", 90},
		{38 * gb, gb + gb/2, "critical", 96.1},
	}
	for _, c := range cases {
		d := NewDiskUsage(c.total, c.free)
		if d == nil || d.Level != c.level || d.UsedPercent != c.pct {
			t.Errorf("NewDiskUsage(%d,%d) = %+v, want level %s at %.1f%%", c.total, c.free, d, c.level, c.pct)
		}
	}
	for _, bad := range [][2]int64{{0, 0}, {-1, 0}, {10, 11}, {10, -1}} {
		if NewDiskUsage(bad[0], bad[1]) != nil {
			t.Errorf("NewDiskUsage(%d,%d) must be rejected", bad[0], bad[1])
		}
	}
}

func TestRecordAndReadRegionDisk(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.db.Exec(`CREATE TABLE regions (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, status TEXT NOT NULL,
		disk_total_bytes INTEGER, disk_free_bytes INTEGER, disk_reported_at DATETIME
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO regions (id, name, status) VALUES ('r1','eu','pending')`); err != nil {
		t.Fatal(err)
	}
	if d, err := s.RegionDisk(ctx, "eu"); err != nil || d != nil {
		t.Fatalf("no reading yet: got %+v, %v", d, err)
	}
	if err := s.RecordDisk(ctx, "eu", 100, 4); err != nil {
		t.Fatal(err)
	}
	d, err := s.RegionDisk(ctx, "eu")
	if err != nil || d == nil || d.Level != "critical" || d.FreeBytes != 4 || d.ReportedAt == nil {
		t.Fatalf("stored reading wrong: %+v, %v", d, err)
	}
	// A nonsense reading must not overwrite a good one.
	_ = s.RecordDisk(ctx, "eu", 0, 0)
	if d2, _ := s.RegionDisk(ctx, "eu"); d2 == nil || d2.FreeBytes != 4 {
		t.Fatalf("bad reading overwrote the stored one: %+v", d2)
	}
}

func TestUnreachableHint(t *testing.T) {
	if UnreachableHint("healthy", nil) != "" {
		t.Error("a healthy region needs no hint")
	}
	plain := UnreachableHint("unreachable", nil)
	if !strings.Contains(plain, "df -h") || strings.Contains(plain, "disk was") {
		t.Errorf("hint without a reading should list causes only: %q", plain)
	}
	full := UnreachableHint("unreachable", NewDiskUsage(100, 3))
	if !strings.HasPrefix(full, "The machine's disk was critical") {
		t.Errorf("a full disk should lead the hint: %q", full)
	}
}
