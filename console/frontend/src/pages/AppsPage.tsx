import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, relativeTime } from '../api'
import { DeployModal } from '../components/DeployModal'
import { EmptyState } from '../components/EmptyState'
import { NewAppModal } from '../components/NewAppModal'
import { PageHeader } from '../components/PageHeader'
import { StatusBadge } from '../components/StatusBadge'
import { useFetch } from '../hooks/useFetch'
import type { Project } from '../types'

export function AppsPage() {
  const navigate = useNavigate()
  const apps = useFetch(async () => (await api<{ projects: Project[] }>('/projects?include=release_status')).projects ?? [], [], 10000)
  const [creating, setCreating] = useState(false)
  const [deploying, setDeploying] = useState<Project | null>(null)

  const list = apps.data ?? []

  return (
    <div className="page-container">
      <PageHeader
        title="Apps"
        action={<button className="btn btn-primary" onClick={() => setCreating(true)}>+ New app</button>}
      />

      {apps.error && <div className="notice notice--error">{apps.error}</div>}

      <div className="card">
        <table className="table">
          <thead>
            <tr><th>Name</th><th>Template</th><th>Status</th><th>Region</th><th>Updated</th><th /></tr>
          </thead>
          <tbody>
            {apps.loading && [1, 2, 3].map((i) => (
              <tr key={i}>{[120, 90, 70, 80, 60].map((w, j) => <td key={j}><div className="skeleton" style={{ height: 14, width: w }} /></td>)}<td /></tr>
            ))}
            {!apps.loading && list.length === 0 && (
              <tr>
                <td colSpan={6}>
                  <EmptyState icon="○" title="No apps yet" hint="Create one here, or ask the agent: “create a java web service called hello-api”" />
                </td>
              </tr>
            )}
            {list.map((p) => (
              <tr key={p.id} className="row-link" onClick={() => navigate(`/apps/${p.name}`)}>
                <td style={{ fontWeight: 500 }}>{p.name}</td>
                <td className="muted">{p.template || '—'}</td>
                <td><StatusBadge status={p.release_status || 'unknown'} /></td>
                <td className="muted">{p.release_region || '—'}</td>
                <td className="muted">{relativeTime(p.updated_at)}</td>
                <td onClick={(e) => e.stopPropagation()} style={{ textAlign: 'right' }}>
                  <button className="btn btn-secondary btn-sm" onClick={() => setDeploying(p)}>Deploy</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {creating && (
        <NewAppModal
          onClose={() => setCreating(false)}
          onCreated={(p) => {
            void apps.reload()
            navigate(`/apps/${p.name}/builds`)
          }}
        />
      )}
      {deploying && (
        <DeployModal
          project={deploying}
          onClose={() => setDeploying(null)}
          onDeployed={() => void apps.reload()}
        />
      )}
    </div>
  )
}
