import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Project } from '../types'
import { useAuth } from '../hooks/useAuth'
import { StatusBadge } from '../components/StatusBadge'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'

function relativeTime(isoDate: string): string {
  const diff = Math.floor((Date.now() - new Date(isoDate).getTime()) / 1000)
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

function SkeletonRow() {
  return (
    <tr>
      <td><div className="skeleton" style={{ height: '14px', width: '120px' }} /></td>
      <td><div className="skeleton" style={{ height: '14px', width: '70px' }} /></td>
      <td><div className="skeleton" style={{ height: '14px', width: '80px' }} /></td>
      <td><div className="skeleton" style={{ height: '14px', width: '60px' }} /></td>
      <td></td>
    </tr>
  )
}

interface ActionsDropdownProps {
  project: Project
  onDeploy: (p: Project) => void
  onDelete: (p: Project) => void
  onRestart: (p: Project) => void
}

function ActionsDropdown({ project, onDeploy, onDelete, onRestart }: ActionsDropdownProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  return (
    <div className="dropdown" ref={ref}>
      <button
        className="btn btn-secondary btn-sm"
        onClick={() => setOpen((o) => !o)}
      >
        Actions ▾
      </button>
      {open && (
        <div className="dropdown-menu">
          <button className="dropdown-item" onClick={() => { onDeploy(project); setOpen(false) }}>Deploy</button>
          <button className="dropdown-item" onClick={() => { onRestart(project); setOpen(false) }}>Restart</button>
          <button
            className="dropdown-item dropdown-item--danger"
            onClick={() => { onDelete(project); setOpen(false) }}
          >
            Delete
          </button>
        </div>
      )}
    </div>
  )
}

interface Props {
  onSendMessage?: (msg: string) => void
}

export function ServicesPage({ onSendMessage }: Props) {
  const { token } = useAuth()
  const navigate = useNavigate()
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
    fetch('/api/v1/projects?include=release_status', { headers })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json() as Promise<{ projects: Project[] }>
      })
      .then((data) => {
        setProjects(data.projects ?? [])
      })
      .catch(() => {
        // Use mock data on error
        setProjects([])
        setError(null)
      })
      .finally(() => setLoading(false))
  }, [token])

  const handleNewService = () => {
    if (onSendMessage) onSendMessage('Create a new service for me')
  }

  const handleDeploy = (p: Project) => {
    if (onSendMessage) onSendMessage(`Deploy service ${p.name}`)
  }

  const handleRestart = (p: Project) => {
    if (onSendMessage) onSendMessage(`Restart service ${p.name}`)
  }

  const handleDelete = (p: Project) => {
    if (onSendMessage) onSendMessage(`Delete service ${p.name}`)
  }

  return (
    <div className="page-container">
      <PageHeader
        title="Services"
        action={
          <button className="btn btn-primary" onClick={handleNewService}>
            + New Service
          </button>
        }
      />

      {error && (
        <div style={{ color: 'var(--status-failed)', marginBottom: '16px', fontSize: '13px' }}>{error}</div>
      )}

      <div className="card">
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Status</th>
              <th>Region</th>
              <th>Updated</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {loading && (
              <>
                <SkeletonRow />
                <SkeletonRow />
                <SkeletonRow />
              </>
            )}
            {!loading && projects.length === 0 && (
              <tr>
                <td colSpan={5}>
                  <EmptyState
                    icon="○"
                    title="No services yet"
                    hint="Ask the agent to deploy one"
                  />
                </td>
              </tr>
            )}
            {!loading && projects.map((p) => (
              <tr
                key={p.id}
                style={{ cursor: 'pointer' }}
                onClick={() => navigate(`/services/${p.name}`)}
              >
                <td style={{ fontWeight: 500 }}>{p.name}</td>
                <td>
                  <StatusBadge status={p.release_status ?? p.status ?? 'unknown'} />
                </td>
                <td style={{ color: 'var(--text-muted)' }}>{p.release_region ?? '—'}</td>
                <td style={{ color: 'var(--text-muted)' }}>{relativeTime(p.updated_at)}</td>
                <td onClick={(e) => e.stopPropagation()}>
                  <ActionsDropdown
                    project={p}
                    onDeploy={handleDeploy}
                    onDelete={handleDelete}
                    onRestart={handleRestart}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
