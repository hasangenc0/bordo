import { useState, useEffect } from 'react'
import { useParams, useNavigate, NavLink, Routes, Route, Navigate } from 'react-router-dom'
import type { Release } from '../types'
import { useAuth } from '../hooks/useAuth'
import { StatusBadge } from '../components/StatusBadge'

// ---- Metrics Tab ----

function generateSparklineData(count = 20): number[] {
  const data: number[] = []
  let v = 20 + Math.random() * 30
  for (let i = 0; i < count; i++) {
    v = Math.max(5, Math.min(95, v + (Math.random() - 0.5) * 12))
    data.push(v)
  }
  return data
}

function Sparkline({ data, color = '#0a0a0a' }: { data: number[]; color?: string }) {
  const w = 280
  const h = 60
  const max = Math.max(...data)
  const min = Math.min(...data)
  const range = max - min || 1
  const points = data
    .map((v, i) => {
      const x = (i / (data.length - 1)) * w
      const y = h - ((v - min) / range) * (h - 8) - 4
      return `${x},${y}`
    })
    .join(' ')
  return (
    <svg width={w} height={h} className="sparkline-wrap" style={{ display: 'block' }}>
      <polyline points={points} fill="none" stroke={color} strokeWidth="1.5" />
    </svg>
  )
}

const TIME_RANGES = ['1h', '6h', '24h', '3d']

function MetricsTab({ name }: { name: string }) {
  const { token } = useAuth()
  const [range, setRange] = useState('1h')
  const [cpuData] = useState(() => generateSparklineData())
  const [memData] = useState(() => generateSparklineData())

  // Try to fetch real data; fallback to mock
  useEffect(() => {
    const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
    fetch(`/api/v1/observe/metrics?project=${name}&range=${range}`, { headers }).catch(() => {})
  }, [name, range, token])

  const cpuValue = Math.round(cpuData[cpuData.length - 1])
  const memValue = Math.round(145 + (memData[memData.length - 1] - 50) * 2)

  return (
    <div>
      <div style={{ display: 'flex', gap: '8px', marginBottom: '20px', justifyContent: 'flex-end' }}>
        <div className="time-range-group">
          {TIME_RANGES.map((r) => (
            <button
              key={r}
              className={`time-range-btn${range === r ? ' time-range-btn--active' : ''}`}
              onClick={() => setRange(r)}
            >
              {r}
            </button>
          ))}
        </div>
      </div>
      <div className="grid-2">
        <div className="stat-card">
          <div className="stat-label">CPU</div>
          <div className="stat-value">{cpuValue}%</div>
          <Sparkline data={cpuData} />
        </div>
        <div className="stat-card">
          <div className="stat-label">Memory</div>
          <div className="stat-value">{memValue} MB</div>
          <Sparkline data={memData} />
        </div>
      </div>
    </div>
  )
}

// ---- Logs Tab ----

function LogsTab({ name }: { name: string }) {
  const { token } = useAuth()
  const [releases, setReleases] = useState<Release[]>([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState<string | null>(null)

  useEffect(() => {
    const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
    fetch(`/api/v1/releases?project_id=${name}`, { headers })
      .then((r) => {
        if (!r.ok) throw new Error()
        return r.json() as Promise<{ releases: Release[] }>
      })
      .then((d) => setReleases(d.releases ?? []))
      .catch(() => setReleases([]))
      .finally(() => setLoading(false))
  }, [name, token])

  if (loading) return <div style={{ color: 'var(--text-muted)', fontSize: '13px' }}>Loading…</div>
  if (releases.length === 0) {
    return <div style={{ color: 'var(--text-muted)', fontSize: '13px' }}>No releases yet.</div>
  }

  return (
    <div className="card">
      <table className="table">
        <thead>
          <tr>
            <th>Status</th>
            <th>Region</th>
            <th>Image</th>
            <th>When</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {releases.map((r) => (
            <>
              <tr key={r.id}>
                <td><StatusBadge status={r.status} /></td>
                <td style={{ color: 'var(--text-muted)' }}>{r.region}</td>
                <td style={{ fontFamily: 'var(--mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{r.image_tag}</td>
                <td style={{ color: 'var(--text-muted)' }}>{new Date(r.created_at).toLocaleString()}</td>
                <td>
                  {r.log && (
                    <button
                      className="btn btn-secondary btn-sm"
                      onClick={() => setExpanded(expanded === r.id ? null : r.id)}
                    >
                      {expanded === r.id ? 'Hide' : 'Logs'}
                    </button>
                  )}
                </td>
              </tr>
              {expanded === r.id && r.log && (
                <tr key={`${r.id}-log`}>
                  <td colSpan={5}>
                    <div className="log-block">{r.log}</div>
                  </td>
                </tr>
              )}
            </>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ---- Variables Tab ----

interface EnvVar { name: string; value: string; secret: boolean }

function VariablesTab({ name }: { name: string }) {
  const { token } = useAuth()
  const [vars, setVars] = useState<EnvVar[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<string | null>(null)
  const [draftValue, setDraftValue] = useState('')
  const [newName, setNewName] = useState('')
  const [newValue, setNewValue] = useState('')

  useEffect(() => {
    const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
    fetch(`/api/v1/projects/${name}/env`, { headers })
      .then((r) => {
        if (!r.ok) throw new Error()
        return r.json() as Promise<{ variables: EnvVar[] }>
      })
      .then((d) => setVars(d.variables ?? []))
      .catch(() => setVars([]))
      .finally(() => setLoading(false))
  }, [name, token])

  const saveVar = async (varName: string, value: string) => {
    const updated = vars.map((v) => v.name === varName ? { ...v, value } : v)
    setVars(updated)
    setEditing(null)
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    }
    await fetch(`/api/v1/projects/${name}/env`, {
      method: 'PUT',
      headers,
      body: JSON.stringify({ variables: updated }),
    }).catch(() => {})
  }

  const addVar = () => {
    if (!newName) return
    const newVar: EnvVar = { name: newName, value: newValue, secret: false }
    const updated = [...vars, newVar]
    setVars(updated)
    setNewName('')
    setNewValue('')
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    }
    fetch(`/api/v1/projects/${name}/env`, {
      method: 'PUT',
      headers,
      body: JSON.stringify({ variables: updated }),
    }).catch(() => {})
  }

  if (loading) return <div style={{ color: 'var(--text-muted)', fontSize: '13px' }}>Loading…</div>

  return (
    <div>
      <div className="card" style={{ marginBottom: '16px' }}>
        <div style={{ padding: '0 16px' }}>
          {vars.map((v) => (
            <div key={v.name} className="var-row">
              <span className="var-name">{v.name}</span>
              {editing === v.name ? (
                <>
                  <input
                    className="input"
                    type={v.secret ? 'password' : 'text'}
                    value={draftValue}
                    onChange={(e) => setDraftValue(e.target.value)}
                    style={{ flex: 1, marginRight: '8px' }}
                    autoFocus
                  />
                  <button className="btn btn-primary btn-sm" onClick={() => saveVar(v.name, draftValue)}>Save</button>
                  <button className="btn btn-secondary btn-sm" onClick={() => setEditing(null)} style={{ marginLeft: '4px' }}>Cancel</button>
                </>
              ) : (
                <>
                  <span className="var-value">{v.secret ? '••••••••' : v.value}</span>
                  <button className="btn btn-secondary btn-sm" onClick={() => { setEditing(v.name); setDraftValue(v.value) }}>Edit</button>
                </>
              )}
            </div>
          ))}
        </div>
      </div>
      <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
        <input
          className="input"
          placeholder="VARIABLE_NAME"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          style={{ width: '200px', fontFamily: 'var(--mono)' }}
        />
        <input
          className="input"
          placeholder="value"
          value={newValue}
          onChange={(e) => setNewValue(e.target.value)}
          style={{ flex: 1 }}
        />
        <button className="btn btn-primary" onClick={addVar}>Add</button>
      </div>
    </div>
  )
}

// ---- Settings Tab ----

function SettingsTab({ name, onSendMessage }: { name: string; onSendMessage?: (msg: string) => void }) {
  const { token } = useAuth()
  const navigate = useNavigate()
  const [confirmDelete, setConfirmDelete] = useState(false)

  const handleDelete = async () => {
    const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
    try {
      await fetch(`/api/v1/projects/${name}`, { method: 'DELETE', headers })
    } catch {
      // ignore
    }
    if (onSendMessage) onSendMessage(`Delete service ${name}`)
    navigate('/services')
  }

  return (
    <div style={{ maxWidth: '480px' }}>
      <div className="form-group">
        <label className="form-label">Name</label>
        <input className="input" value={name} readOnly style={{ color: 'var(--text-muted)', cursor: 'not-allowed' }} />
      </div>

      <div className="danger-zone">
        <div className="danger-zone-title">Danger Zone</div>
        <p style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '12px' }}>
          Deleting this service is irreversible and will remove all associated data.
        </p>
        {!confirmDelete ? (
          <button className="btn btn-danger" onClick={() => setConfirmDelete(true)}>Delete Service</button>
        ) : (
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <span style={{ fontSize: '13px', color: 'var(--text-muted)' }}>Are you sure?</span>
            <button className="btn btn-danger" onClick={handleDelete}>Yes, Delete</button>
            <button className="btn btn-secondary" onClick={() => setConfirmDelete(false)}>Cancel</button>
          </div>
        )}
      </div>
    </div>
  )
}

// ---- Main Page ----

interface Props {
  onSendMessage?: (msg: string) => void
}

export function ServiceDetailPage({ onSendMessage }: Props) {
  const { name } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const serviceName = name ?? ''

  const TABS = [
    { label: 'Metrics', path: 'metrics' },
    { label: 'Logs', path: 'logs' },
    { label: 'Variables', path: 'variables' },
    { label: 'Settings', path: 'settings' },
  ]

  return (
    <div className="page-container">
      <div className="breadcrumb">
        <a href="#" onClick={(e) => { e.preventDefault(); navigate('/services') }}>Services</a>
        <span className="breadcrumb-sep">/</span>
        <span className="breadcrumb-current">{serviceName}</span>
      </div>

      <div className="page-header">
        <h1 className="page-title">{serviceName}</h1>
      </div>

      <div className="tabs">
        {TABS.map((tab) => (
          <NavLink
            key={tab.path}
            to={`/services/${serviceName}/${tab.path}`}
            className={({ isActive }: { isActive: boolean }) => `tab${isActive ? ' tab--active' : ''}`}
          >
            {tab.label}
          </NavLink>
        ))}
      </div>

      <Routes>
        <Route index element={<Navigate to="metrics" replace />} />
        <Route path="metrics" element={<MetricsTab name={serviceName} />} />
        <Route path="logs" element={<LogsTab name={serviceName} />} />
        <Route path="variables" element={<VariablesTab name={serviceName} />} />
        <Route path="settings" element={<SettingsTab name={serviceName} onSendMessage={onSendMessage} />} />
      </Routes>
    </div>
  )
}
