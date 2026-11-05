package fleet

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// Disk usage levels. Kubelet starts evicting pods around 85-90% used, so the platform warns well
// before that.
const (
	DiskWarnPercent     = 80.0
	DiskCriticalPercent = 90.0
)

// DiskUsage is the usage of the filesystem a machine keeps its containers and data on.
type DiskUsage struct {
	TotalBytes  int64   `json:"total_bytes"`
	FreeBytes   int64   `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
	// Level is "ok", "warn" or "critical".
	Level      string     `json:"level"`
	ReportedAt *time.Time `json:"reported_at,omitempty"`
}

// NewDiskUsage derives the used percentage and level from a total and free byte count. It returns
// nil when the numbers are not a usable reading.
func NewDiskUsage(total, free int64) *DiskUsage {
	if total <= 0 || free < 0 || free > total {
		return nil
	}
	used := math.Round(float64(total-free)/float64(total)*1000) / 10
	level := "ok"
	switch {
	case used >= DiskCriticalPercent:
		level = "critical"
	case used >= DiskWarnPercent:
		level = "warn"
	}
	return &DiskUsage{TotalBytes: total, FreeBytes: free, UsedPercent: used, Level: level}
}

// Describe renders the reading for humans, e.g. "96% used, 1.5 GB free of 38 GB".
func (d *DiskUsage) Describe() string {
	return fmt.Sprintf("%.0f%% used, %s free of %s", d.UsedPercent, humanBytes(d.FreeBytes), humanBytes(d.TotalBytes))
}

func humanBytes(n int64) string {
	const gb = 1 << 30
	const mb = 1 << 20
	if n >= gb {
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	}
	return fmt.Sprintf("%.0f MB", float64(n)/mb)
}

// RecordDisk stores a region's latest disk reading. An unusable reading is ignored.
func (s *Store) RecordDisk(ctx context.Context, region string, total, free int64) error {
	if NewDiskUsage(total, free) == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE regions SET disk_total_bytes = ?, disk_free_bytes = ?, disk_reported_at = ? WHERE name = ?`,
		total, free, time.Now().UTC(), region)
	return err
}

// RegionDisk returns a region's last reported disk usage, or nil when it has never reported one.
func (s *Store) RegionDisk(ctx context.Context, region string) (*DiskUsage, error) {
	var (
		total, free sql.NullInt64
		at          sql.NullTime
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT disk_total_bytes, disk_free_bytes, disk_reported_at FROM regions WHERE name = ?`, region).
		Scan(&total, &free, &at)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return diskFromRow(total, free, at), nil
}

// DiskFromRow builds a DiskUsage from nullable columns, or nil when no reading was stored.
func DiskFromRow(total, free sql.NullInt64, at sql.NullTime) *DiskUsage {
	return diskFromRow(total, free, at)
}

func diskFromRow(total, free sql.NullInt64, at sql.NullTime) *DiskUsage {
	if !total.Valid || !free.Valid {
		return nil
	}
	d := NewDiskUsage(total.Int64, free.Int64)
	if d != nil && at.Valid {
		t := at.Time
		d.ReportedAt = &t
	}
	return d
}

// UnreachableHint explains, for a region whose reconciler has stopped pulling, what usually
// causes that and how to tell which it is. Disk pressure is the case that is easy to miss (the
// node is Ready but refuses new pods), so a last-known reading is used to point at it first.
// It returns "" for a region that is not unreachable.
func UnreachableHint(status string, disk *DiskUsage) string {
	if status != "unreachable" {
		return ""
	}
	var b strings.Builder
	if disk != nil && disk.Level != "ok" {
		fmt.Fprintf(&b, "The machine's disk was %s the last time it reported (%s). A full disk makes the node refuse new pods, so the reconciler cannot restart. Free space, then check `kubectl describe node | grep -i pressure`. ", disk.Level, disk.Describe())
	}
	b.WriteString("The reconciler has stopped pulling. Likely causes: the machine is offline, the node is out of disk or memory (run `df -h` and `kubectl describe node | grep -i pressure` on it), the reconciler pod is not running (`kubectl -n bordo-system get pods`), or the control plane URL it was given is no longer reachable from the machine (see `kubectl -n bordo-system logs deploy/bordo-reconciler`). Re-running the join command is only needed if the logs show an invalid cluster token.")
	return b.String()
}
