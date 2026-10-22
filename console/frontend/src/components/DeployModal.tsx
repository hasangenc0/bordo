import { useState } from 'react'
import { api, listBuilds, listRegions } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { Project } from '../types'
import { Modal } from './Modal'

interface Props {
  project: Project
  onClose: () => void
  onDeployed: () => void
}

export function DeployModal({ project, onClose, onDeployed }: Props) {
  const regions = useFetch(listRegions, [])
  const builds = useFetch(() => listBuilds(project.id), [project.id])

  const [picked, setPicked] = useState<string[]>([])
  const [image, setImage] = useState('')
  const [strategy, setStrategy] = useState('rolling')
  const [expose, setExpose] = useState('none')
  const [host, setHost] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const built = (builds.data ?? []).filter((b) => b.status === 'success' && b.image_tag)
  const readyRegions = (regions.data ?? []).filter((r) => r.status === 'healthy')
  const effectiveImage = image || built[0]?.image_tag || ''

  const toggle = (name: string) =>
    setPicked((p) => (p.includes(name) ? p.filter((n) => n !== name) : [...p, name]))

  const problem =
    picked.length === 0 ? 'Pick at least one region.'
    : !effectiveImage ? 'Choose or enter an image to deploy.'
    : expose === 'ingress' && !host.trim() ? 'Ingress needs a hostname.'
    : null

  const submit = async () => {
    if (problem) return
    setSubmitting(true)
    setError(null)
    try {
      await api('/releases', {
        json: {
          project_id: project.id,
          image_tag: effectiveImage,
          regions: picked,
          strategy,
          expose,
          ...(expose === 'ingress' ? { expose_host: host.trim() } : {}),
        },
      })
      onDeployed()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Deploy failed')
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={`Deploy ${project.name}`}
      onClose={onClose}
      footer={
        <>
          <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={() => void submit()} disabled={!!problem || submitting}>
            {submitting ? 'Deploying…' : picked.length ? `Deploy to ${picked.length} region${picked.length === 1 ? '' : 's'}` : 'Deploy'}
          </button>
        </>
      }
    >
      <div className="form-group">
        <label className="form-label">Regions</label>
        {regions.loading && <div className="muted">Loading regions…</div>}
        {!regions.loading && (regions.data ?? []).length === 0 && (
          <div className="notice">No regions yet. Add one on the Regions page, then come back.</div>
        )}
        <div className="check-list">
          {(regions.data ?? []).map((r) => {
            const ok = r.status === 'healthy'
            return (
              <label key={r.id || r.name} className={`check-row${ok ? '' : ' check-row--disabled'}`}>
                <input type="checkbox" checked={picked.includes(r.name)} disabled={!ok} onChange={() => toggle(r.name)} />
                <span>{r.name}</span>
                <span className={`status-text status-text--${r.status}`}>{r.status}</span>
              </label>
            )
          })}
        </div>
        {(regions.data ?? []).length > 0 && readyRegions.length === 0 && (
          <div className="notice">No region is healthy yet. A region becomes healthy once its machine has joined.</div>
        )}
      </div>

      <div className="form-group">
        <label className="form-label">Image</label>
        {built.length > 0 ? (
          <select className="select" value={effectiveImage} onChange={(e) => setImage(e.target.value)}>
            {built.map((b) => (
              <option key={b.id} value={b.image_tag}>{b.image_tag}</option>
            ))}
          </select>
        ) : (
          <>
            <input className="input" placeholder="ghcr.io/you/app:tag" value={image} onChange={(e) => setImage(e.target.value)} />
            {!builds.loading && <div className="muted" style={{ marginTop: 6 }}>No successful builds yet. Build one from the Builds tab, or enter an image.</div>}
          </>
        )}
      </div>

      <div className="form-row">
        <div className="form-group">
          <label className="form-label">Strategy</label>
          <select className="select" value={strategy} onChange={(e) => setStrategy(e.target.value)}>
            <option value="rolling">Rolling</option>
            <option value="canary">Canary</option>
            <option value="blue-green">Blue / green</option>
          </select>
        </div>
        <div className="form-group">
          <label className="form-label">Public access</label>
          <select className="select" value={expose} onChange={(e) => setExpose(e.target.value)}>
            <option value="none">Internal only</option>
            <option value="nodeport">Public port (NodePort)</option>
            <option value="ingress">Hostname (Ingress)</option>
          </select>
        </div>
      </div>
      {expose === 'ingress' && (
        <div className="form-group">
          <label className="form-label">Hostname</label>
          <input className="input" placeholder="app.example.com" value={host} onChange={(e) => setHost(e.target.value)} />
        </div>
      )}

      {error && <div className="notice notice--error">{error}</div>}
      {!error && problem && picked.length + (effectiveImage ? 1 : 0) > 0 && <div className="muted">{problem}</div>}
    </Modal>
  )
}
