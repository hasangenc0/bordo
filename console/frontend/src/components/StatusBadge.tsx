interface Props {
  status: string
  size?: 'sm' | 'md'
}

// Control-plane statuses (releases, builds, regions) mapped onto the visual states.
const CANONICAL: Record<string, string> = {
  deployed: 'running',
  success: 'running',
  succeeded: 'running',
  healthy: 'running',
  ready: 'running',
  reconciling: 'deploying',
  rolling_back: 'deploying',
  bootstrapping: 'deploying',
  building: 'deploying',
  running_build: 'deploying',
  unreachable: 'failed',
  error: 'failed',
}

const LABELS: Record<string, string> = {
  running: 'Running',
  deployed: 'Running',
  deploying: 'Deploying',
  reconciling: 'Deploying',
  rolling_back: 'Rolling back',
  failed: 'Failed',
  warning: 'Warning',
  unknown: 'Not deployed',
  auto_approved: 'Auto',
  approved: 'Approved',
  pending: 'Pending',
  success: 'Succeeded',
  healthy: 'Healthy',
  bootstrapping: 'Joining',
  unreachable: 'Unreachable',
}

export function StatusBadge({ status, size = 'md' }: Props) {
  const key = (status || 'unknown').toLowerCase().replace(/\s+/g, '_')
  const label = LABELS[key] ?? status
  const cls = CANONICAL[key] ?? key
  return (
    <span className={`status-badge status-badge--${cls}`} style={size === 'sm' ? { fontSize: '11px' } : {}}>
      <span className="status-badge-dot" />
      {label}
    </span>
  )
}
