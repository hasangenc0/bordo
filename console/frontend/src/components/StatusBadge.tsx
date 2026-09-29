interface Props {
  status: string
  size?: 'sm' | 'md'
}

const STATUS_LABELS: Record<string, string> = {
  running: 'Running',
  deploying: 'Deploying',
  failed: 'Failed',
  warning: 'Warning',
  unknown: 'Unknown',
  auto_approved: 'Auto',
  approved: 'Approved',
  pending: 'Pending',
}

export function StatusBadge({ status, size = 'md' }: Props) {
  const label = STATUS_LABELS[status] ?? status
  const cls = status.toLowerCase().replace(/\s+/g, '_')
  return (
    <span className={`status-badge status-badge--${cls}`} style={size === 'sm' ? { fontSize: '11px' } : {}}>
      <span className="status-badge-dot" />
      {label}
    </span>
  )
}
