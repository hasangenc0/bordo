import { Fragment, useState } from 'react'
import { Navigate, NavLink, Route, Routes, useNavigate, useParams } from 'react-router-dom'
import { api, listBuilds, relativeTime } from '../api'
import { DeployModal } from '../components/DeployModal'
import { LineChart } from '../components/LineChart'
import { StatusBadge } from '../components/StatusBadge'
import { useFetch } from '../hooks/useFetch'
import { APP_PRESETS, appLogSelector, queryLogs, queryRange, RANGES } from '../lib/metrics'
import type { EnvVar, Project, Release } from '../types'

// ---- Metrics ----

function MetricsTab({ name }: { name: string }) {
  const [range, setRange] = useState(RANGES[0])
  const res = useFetch(
    () => Promise.all(APP_PRESETS.map((p) => queryRange(p.query(name), range))),
    [name, range.label],
    30000,
  )
  const results = res.data
  const unavailable = results?.find((r) => r.unavailable)?.unavailable
  const errors = [...new Set((results ?? []).flatMap((r) => r.errors))]
  return (
    <div>
      <div className="toolbar">
        <div className="time-range-group">
          {RANGES.map((r) => (
            <button key={r.label} className={`time-range-btn${range.label === r.label ? ' time-range-btn--active' : ''}`} onClick={() => setRange(r)}>
              {r.label}
            </button>
          ))}
        </div>
      </div>
      {res.error && <div className="notice notice--error">{res.error}</div>}
      {unavailable && (
        <div className="notice">
          Observability isn't set up on this server yet, so there's nothing to chart. Deploy the observability stack and
          point each region at it; see the Observe page for the backend settings.
        </div>
      )}
      {errors.map((e) => <div key={e} className="notice notice--error">{e}</div>)}
      {!unavailable && (
        <div className="grid-2">
          {APP_PRESETS.map((p, i) => (
            <div key={p.id} className="stat-card">
              <div className="stat-label">{p.title}</div>
              {!results ? <div className="skeleton" style={{ height: 120 }} /> : <LineChart series={results[i].series} format={p.format} height={150} />}
            </div>
          ))}
        </div>
      )}
      <p className="muted" style={{ marginTop: 16 }}>
        These charts read the metrics your app exports over OpenTelemetry. For anything else, use the{' '}
        <NavLink to="/observe">Observe</NavLink> page to run your own query.
      </p>
    </div>
  )
}

// ---- Logs ----

function LogsTab({ name }: { name: string }) {
  const [filter, setFilter] = useState('')
  const res = useFetch(() => queryLogs(appLogSelector(name), 3600), [name], 10000)
  const lines = (res.data?.lines ?? []).filter((l) => !filter || l.line.toLowerCase().includes(filter.toLowerCase()))
  return (
    <div>
      <div className="toolbar">
        <input className="input" style={{ maxWidth: 280 }} placeholder="Filter lines…" value={filter} onChange={(e) => setFilter(e.target.value)} />
        <span className="muted">Last hour · refreshes every 10s</span>
      </div>
      {res.error && <div className="notice notice--error">{res.error}</div>}
      {res.data?.unavailable && <div className="notice">Observability isn't set up on this server yet, so there are no logs to show.</div>}
      {res.data?.errors.map((e) => <div key={e} className="notice notice--error">{e}</div>)}
      {!res.loading && !res.data?.unavailable && lines.length === 0 && !res.error && (
        <div className="muted">No log lines in the last hour.</div>
      )}
      {lines.length > 0 && (
        <div className="log-block">
          {lines.map((l, i) => (
            <div key={i}><span className="log-ts">{new Date(l.ts).toLocaleTimeString()}</span> {l.line}</div>
          ))}
        </div>
      )}
    </div>
  )
}

// ---- Deployments ----

function DeploymentsTab({ project, onDeploy }: { project: Project; onDeploy: () => void }) {
  const rels = useFetch(
    async () => (await api<{ releases: Release[] }>(`/releases?project_id=${project.id}`)).releases ?? [],
    [project.id],
    5000,
  )
  const [open, setOpen] = useState<string | null>(null)
  const [log, setLog] = useState<string>('')
  const [confirm, setConfirm] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const showLog = async (id: string) => {
    if (open === id) return setOpen(null)
    setOpen(id)
    setLog('Loading…')
    try {
      setLog((await api<{ log: string }>(`/releases/${id}/logs`)).log || 'No output yet.')
    } catch (e) {
      setLog(e instanceof Error ? e.message : 'Could not load logs')
    }
  }

  const rollback = async (id: string) => {
    setError(null)
    try {
      await api(`/releases/${id}/rollback`, { json: {} })
      setConfirm(null)
      void rels.reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Rollback failed')
    }
  }

  const list = rels.data ?? []
  return (
    <div>
      <div className="toolbar"><button className="btn btn-primary" onClick={onDeploy}>Deploy…</button></div>
      {error && <div className="notice notice--error">{error}</div>}
      {!rels.loading && list.length === 0 ? (
        <div className="muted">Nothing deployed yet. Build the app, then press Deploy.</div>
      ) : (
        <div className="card">
          <table className="table">
            <thead><tr><th>Status</th><th>Region</th><th>Image</th><th>Strategy</th><th>When</th><th /></tr></thead>
            <tbody>
              {list.map((r) => (
                <Fragment key={r.id}>
                  <tr>
                    <td><StatusBadge status={r.status} /></td>
                    <td className="muted">{r.region}</td>
                    <td className="mono muted ellipsis" title={r.image_tag}>{r.image_tag}</td>
                    <td className="muted">{r.strategy || 'rolling'}</td>
                    <td className="muted">{relativeTime(r.created_at)}</td>
                    <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                      <button className="btn btn-secondary btn-sm" onClick={() => void showLog(r.id)}>{open === r.id ? 'Hide' : 'Logs'}</button>{' '}
                      {confirm === r.id ? (
                        <>
                          <button className="btn btn-danger btn-sm" onClick={() => void rollback(r.id)}>Confirm rollback</button>{' '}
                          <button className="btn btn-secondary btn-sm" onClick={() => setConfirm(null)}>Cancel</button>
                        </>
                      ) : (
                        <button className="btn btn-secondary btn-sm" onClick={() => setConfirm(r.id)}>Roll back</button>
                      )}
                    </td>
                  </tr>
                  {open === r.id && (
                    <tr><td colSpan={6}><div className="log-block">{log}</div></td></tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

// ---- Builds ----

function BuildsTab({ project }: { project: Project }) {
  const builds = useFetch(() => listBuilds(project.id), [project.id], 5000)
  const [open, setOpen] = useState<string | null>(null)
  const [log, setLog] = useState('')
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmEnable, setConfirmEnable] = useState(false)
  const [enabled, setEnabled] = useState<string | null>(null)

  const enableBuilds = async () => {
    setError(null)
    try {
      const r = await api<{ repo: string; workflow: string }>('/builds/enable', { json: { project_id: project.id } })
      setEnabled(`Added ${r.workflow} to ${r.repo}. Build again once it has been committed.`)
      setConfirmEnable(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not add the workflow')
    }
  }

  const start = async () => {
    setStarting(true)
    setError(null)
    try {
      await api('/builds', { json: { project_id: project.id } })
      void builds.reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not start build')
    } finally {
      setStarting(false)
    }
  }

  const showLog = async (id: string) => {
    if (open === id) return setOpen(null)
    setOpen(id)
    setLog('Loading…')
    try {
      const { lines } = await api<{ lines: unknown[] }>(`/builds/${id}/logs`)
      setLog((lines ?? []).map((l) => (typeof l === 'string' ? l : JSON.stringify(l))).join('\n') || 'No output yet.')
    } catch (e) {
      setLog(e instanceof Error ? e.message : 'Could not load logs')
    }
  }

  const list = builds.data ?? []
  const needsWorkflow = !enabled && !!project.git_repo_url && !project.template && !!list[0]?.error_message?.includes('Bordo build workflow')
  return (
    <div>
      <div className="toolbar">
        <button className="btn btn-primary" onClick={() => void start()} disabled={starting}>{starting ? 'Starting…' : 'Build now'}</button>
      </div>
      {error && <div className="notice notice--error">{error}</div>}
      {enabled && <div className="notice">{enabled}</div>}
      {needsWorkflow && (
        <div className="notice">
          <strong>This repo has no Bordo build workflow.</strong> Bordo can add <code>.github/workflows/bordo-build.yml</code> with a single
          commit to the repo's default branch. It won't change your repo unless you agree.
          <div className="toolbar" style={{ marginTop: 10, marginBottom: 0 }}>
            {!confirmEnable ? (
              <button className="btn btn-secondary btn-sm" onClick={() => setConfirmEnable(true)}>Add build workflow…</button>
            ) : (
              <>
                <button className="btn btn-primary btn-sm" onClick={() => void enableBuilds()}>Yes, commit to the default branch</button>
                <button className="btn btn-secondary btn-sm" onClick={() => setConfirmEnable(false)}>Cancel</button>
              </>
            )}
          </div>
        </div>
      )}
      {!builds.loading && list.length === 0 ? (
        <div className="muted">No builds yet.</div>
      ) : (
        <div className="card">
          <table className="table">
            <thead><tr><th>Status</th><th>Image</th><th>Started</th><th /></tr></thead>
            <tbody>
              {list.map((b) => (
                <Fragment key={b.id}>
                  <tr>
                    <td><StatusBadge status={b.status} /></td>
                    <td className="mono muted ellipsis" title={b.image_tag}>{b.image_tag || '—'}</td>
                    <td className="muted">{relativeTime(b.created_at)}</td>
                    <td style={{ textAlign: 'right' }}>
                      <button className="btn btn-secondary btn-sm" onClick={() => void showLog(b.id)}>{open === b.id ? 'Hide' : 'Logs'}</button>
                    </td>
                  </tr>
                  {b.error_message && (
                    <tr><td colSpan={4} className="err-cell">{b.error_message}</td></tr>
                  )}
                  {open === b.id && <tr><td colSpan={4}><div className="log-block">{log}</div></td></tr>}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

// ---- Code ----

const CODE_IDEAS = [
  'Add a /health endpoint that returns the app version',
  'Add request logging with the request id',
  'Write a README explaining how to run this locally',
  'Find and fix any obvious bugs',
]

function CodeTab({ project, onAsk }: { project: Project; onAsk: (msg: string) => void }) {
  const [text, setText] = useState('')
  const ask = (t: string) => {
    if (!t.trim()) return
    onAsk(
      `In the app "${project.name}": ${t.trim()}\n\nWork on a new branch, show me the diff when you're done, then push it and open a pull request. Don't touch the default branch.`,
    )
    setText('')
  }
  return (
    <div style={{ maxWidth: 640 }}>
      <p className="muted" style={{ marginBottom: 12 }}>
        Describe a change. The agent checks out the code, edits it on its own branch, shows you the diff and opens a pull request.
        Nothing reaches your default branch without your review.
      </p>
      <textarea className="input" rows={4} placeholder="What should change?" value={text} onChange={(e) => setText(e.target.value)} />
      <div className="toolbar" style={{ marginTop: 12 }}>
        <button className="btn btn-primary" onClick={() => ask(text)} disabled={!text.trim()}>Ask the agent</button>
      </div>
      <div className="form-label" style={{ marginTop: 20 }}>Ideas</div>
      <div className="chip-row">
        {CODE_IDEAS.map((i) => <button key={i} className="chip" onClick={() => setText(i)}>{i}</button>)}
      </div>
      {project.git_repo_url && (
        <p className="muted" style={{ marginTop: 20 }}>
          Repository: <a href={project.git_repo_url} target="_blank" rel="noreferrer noopener">{project.git_repo_url}</a>
        </p>
      )}
    </div>
  )
}

// ---- Variables ----

function VariablesTab({ project }: { project: Project }) {
  const vars = useFetch(async () => (await api<{ vars: EnvVar[] }>(`/projects/${project.id}/env`)).vars ?? [], [project.id])
  const [newName, setNewName] = useState('')
  const [newValue, setNewValue] = useState('')
  const [error, setError] = useState<string | null>(null)
  const list = vars.data ?? []
  // The API returns secrets masked and replaces the whole set on save, so saving from here
  // would overwrite real secret values with the mask. Keep this view read-only in that case.
  const hasSecrets = list.some((v) => v.secret)

  const save = async (next: EnvVar[]) => {
    setError(null)
    try {
      await api(`/projects/${project.id}/env`, { method: 'PUT', json: { vars: next } })
      void vars.reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not save')
    }
  }

  const add = async () => {
    const name = newName.trim()
    if (!/^[A-Z_][A-Z0-9_]*$/i.test(name)) return setError('Names use letters, numbers and underscores.')
    await save([...list.filter((v) => v.name !== name), { name, value: newValue, secret: false }])
    setNewName('')
    setNewValue('')
  }

  return (
    <div style={{ maxWidth: 720 }}>
      {error && <div className="notice notice--error">{error}</div>}
      {hasSecrets && (
        <div className="notice">This app has secret variables, so they can't be edited here. Manage them with <code>bordo secret</code> or ask the agent.</div>
      )}
      <div className="card" style={{ marginBottom: 16 }}>
        <div style={{ padding: '0 16px' }}>
          {list.length === 0 && !vars.loading && <div className="muted" style={{ padding: '14px 0' }}>No variables yet.</div>}
          {list.map((v) => (
            <div key={v.name} className="var-row">
              <span className="var-name">{v.name}</span>
              <span className="var-value">{v.secret ? '••••••••' : v.value}</span>
              {!hasSecrets && <button className="btn btn-secondary btn-sm" onClick={() => void save(list.filter((x) => x.name !== v.name))}>Remove</button>}
            </div>
          ))}
        </div>
      </div>
      {!hasSecrets && (
        <div className="toolbar">
          <input className="input" style={{ width: 200, fontFamily: 'var(--mono)' }} placeholder="NAME" value={newName} onChange={(e) => setNewName(e.target.value)} />
          <input className="input" style={{ flex: 1 }} placeholder="value" value={newValue} onChange={(e) => setNewValue(e.target.value)} />
          <button className="btn btn-primary" onClick={() => void add()} disabled={!newName.trim()}>Add</button>
        </div>
      )}
      <p className="muted">Changes apply on the next deploy.</p>
    </div>
  )
}

// ---- Settings ----

function SettingsTab({ project }: { project: Project }) {
  const navigate = useNavigate()
  const [confirm, setConfirm] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const del = async () => {
    try {
      await api(`/projects/${project.id}`, { method: 'DELETE' })
      navigate('/apps')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed')
    }
  }
  return (
    <div style={{ maxWidth: 480 }}>
      <div className="form-group">
        <label className="form-label">Name</label>
        <input className="input" value={project.name} readOnly />
      </div>
      <div className="danger-zone">
        <div className="danger-zone-title">Danger zone</div>
        <p className="muted" style={{ marginBottom: 12 }}>
          Removes this app from Bordo. Running deployments and the GitHub repository are not touched.
        </p>
        {error && <div className="notice notice--error">{error}</div>}
        {!confirm ? (
          <button className="btn btn-danger" onClick={() => setConfirm(true)}>Delete app</button>
        ) : (
          <div className="toolbar">
            <span className="muted">Are you sure?</span>
            <button className="btn btn-danger" onClick={() => void del()}>Yes, delete</button>
            <button className="btn btn-secondary" onClick={() => setConfirm(false)}>Cancel</button>
          </div>
        )}
      </div>
    </div>
  )
}

// ---- Page ----

const TABS = [
  { label: 'Metrics', path: 'metrics' },
  { label: 'Logs', path: 'logs' },
  { label: 'Deployments', path: 'deployments' },
  { label: 'Builds', path: 'builds' },
  { label: 'Code', path: 'code' },
  { label: 'Variables', path: 'variables' },
  { label: 'Settings', path: 'settings' },
]

export function AppDetailPage({ onAsk }: { onAsk: (msg: string) => void }) {
  const { name = '' } = useParams<{ name: string }>()
  const projects = useFetch(async () => (await api<{ projects: Project[] }>('/projects?include=release_status')).projects ?? [], [name], 10000)
  const [deploying, setDeploying] = useState(false)
  const project = (projects.data ?? []).find((p) => p.name === name)

  if (projects.loading && !projects.data) return <div className="page-container"><div className="skeleton" style={{ height: 28, width: 200 }} /></div>
  if (!project) {
    return (
      <div className="page-container">
        <div className="breadcrumb"><NavLink to="/apps">Apps</NavLink></div>
        <p className="muted">{projects.error ?? `No app named “${name}”.`}</p>
      </div>
    )
  }

  return (
    <div className="page-container">
      <div className="breadcrumb">
        <NavLink to="/apps">Apps</NavLink>
        <span className="breadcrumb-sep">/</span>
        <span className="breadcrumb-current">{project.name}</span>
      </div>

      <div className="page-header">
        <div>
          <h1 className="page-title">{project.name}</h1>
          <div className="page-sub">
            <StatusBadge status={project.release_status || 'unknown'} size="sm" />
            {project.release_region && <span className="muted"> in {project.release_region}</span>}
          </div>
        </div>
        <button className="btn btn-primary" onClick={() => setDeploying(true)}>Deploy…</button>
      </div>

      <div className="tabs">
        {TABS.map((t) => (
          <NavLink key={t.path} to={`/apps/${project.name}/${t.path}`} className={({ isActive }: { isActive: boolean }) => `tab${isActive ? ' tab--active' : ''}`}>
            {t.label}
          </NavLink>
        ))}
      </div>

      <Routes>
        <Route index element={<Navigate to="metrics" replace />} />
        <Route path="metrics" element={<MetricsTab name={project.name} />} />
        <Route path="logs" element={<LogsTab name={project.name} />} />
        <Route path="deployments" element={<DeploymentsTab project={project} onDeploy={() => setDeploying(true)} />} />
        <Route path="builds" element={<BuildsTab project={project} />} />
        <Route path="code" element={<CodeTab project={project} onAsk={onAsk} />} />
        <Route path="variables" element={<VariablesTab project={project} />} />
        <Route path="settings" element={<SettingsTab project={project} />} />
      </Routes>

      {deploying && <DeployModal project={project} onClose={() => setDeploying(false)} onDeployed={() => void projects.reload()} />}
    </div>
  )
}
