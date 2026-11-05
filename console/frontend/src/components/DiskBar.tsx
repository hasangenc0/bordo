import type { DiskUsage } from '../types'

function human(bytes: number): string {
  const gb = 1024 ** 3
  return bytes >= gb ? `${(bytes / gb).toFixed(1)} GB` : `${Math.round(bytes / 1024 ** 2)} MB`
}

/** How full a machine's disk is. A full disk is the usual reason a region stops responding. */
export function DiskBar({ disk, label = 'Disk' }: { disk?: DiskUsage | null; label?: string }) {
  if (!disk) {
    return (
      <div className="disk" data-level="unknown">
        <div className="disk-row"><span>{label}</span><span className="muted">not reported yet</span></div>
      </div>
    )
  }
  const pct = Math.min(100, Math.max(0, disk.used_percent))
  return (
    <div className="disk" data-level={disk.level}>
      <div className="disk-row">
        <span>{label}</span>
        <span>
          {Math.round(pct)}% used <span className="muted">· {human(disk.free_bytes)} free of {human(disk.total_bytes)}</span>
        </span>
      </div>
      <div className="disk-track" role="progressbar" aria-label={`${label} usage`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)}>
        <div className="disk-fill" style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}
