import type { ActionEvent } from '../types'
import { StatusBadge } from './StatusBadge'

interface Props {
  events: ActionEvent[]
  onApprove: (id: string) => void
  onReject: (id: string) => void
  onClose: () => void
}

function relativeTime(ts: number): string {
  const diff = Math.floor((Date.now() - ts) / 1000)
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

const KIND_ICONS: Record<string, string> = {
  deploy: '↑',
  delete: '✕',
  restart: '↺',
  build: '⚙',
  rollback: '↩',
}

export function ActivityPanel({ events, onApprove, onReject, onClose }: Props) {
  return (
    <aside className="activity-panel">
      <div className="activity-panel-header">
        <span>Activity</span>
        <button className="activity-toggle" onClick={onClose} style={{ padding: '2px 8px', fontSize: '12px' }}>✕</button>
      </div>
      <div className="activity-panel-list">
        {events.length === 0 && (
          <div style={{ padding: '32px 16px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
            No agent activity yet
          </div>
        )}
        {events.map((ev) => (
          <div key={ev.id} className="activity-item">
            <div className="activity-item-header">
              <span className="activity-item-kind">
                {KIND_ICONS[ev.kind] ?? '●'} {ev.kind} {ev.target}
              </span>
              <StatusBadge status={ev.status} size="sm" />
            </div>
            <div className="activity-item-meta">
              {relativeTime(ev.ts)}
              {ev.log_url && (
                <a
                  href={ev.log_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  style={{ marginLeft: '8px', color: 'var(--text)', textDecoration: 'none' }}
                >
                  View logs →
                </a>
              )}
            </div>
            {ev.status === 'pending' && (
              <div className="pending-actions">
                <button
                  className="btn btn-primary btn-sm"
                  onClick={() => onApprove(ev.id)}
                >
                  Approve
                </button>
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={() => onReject(ev.id)}
                >
                  Reject
                </button>
              </div>
            )}
          </div>
        ))}
      </div>
    </aside>
  )
}
