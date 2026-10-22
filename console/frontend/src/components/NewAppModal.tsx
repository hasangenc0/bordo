import { useState } from 'react'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { Project, Template } from '../types'
import { Modal } from './Modal'

interface Props {
  onClose: () => void
  onCreated: (p: Project) => void
}

const NAME_RE = /^[a-z][a-z0-9-]{1,38}[a-z0-9]$/

export function NewAppModal({ onClose, onCreated }: Props) {
  const templates = useFetch(async () => (await api<{ templates: Template[] }>('/templates')).templates ?? [], [])
  const [name, setName] = useState('')
  const [template, setTemplate] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const list = templates.data ?? []
  const chosen = template || list[0]?.name || ''
  const nameOk = NAME_RE.test(name)

  const submit = async () => {
    if (!nameOk || !chosen) return
    setSubmitting(true)
    setError(null)
    try {
      const p = await api<Project>('/projects', { json: { name, template: chosen } })
      onCreated(p)
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not create app')
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="New app"
      onClose={onClose}
      footer={
        <>
          <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={() => void submit()} disabled={!nameOk || !chosen || submitting}>
            {submitting ? 'Creating…' : 'Create app'}
          </button>
        </>
      }
    >
      <div className="form-group">
        <label className="form-label">Name</label>
        <input className="input" autoFocus placeholder="hello-api" value={name} onChange={(e) => setName(e.target.value.toLowerCase())} />
        {name && !nameOk && <div className="muted" style={{ marginTop: 6 }}>Use 3–40 lowercase letters, numbers and dashes, starting with a letter.</div>}
      </div>
      <div className="form-group">
        <label className="form-label">Template</label>
        {templates.loading && <div className="muted">Loading templates…</div>}
        {templates.error && <div className="notice notice--error">{templates.error}</div>}
        <div className="option-list">
          {list.map((t) => (
            <label key={t.name} className={`option${chosen === t.name ? ' option--selected' : ''}`}>
              <input type="radio" name="template" checked={chosen === t.name} onChange={() => setTemplate(t.name)} />
              <div>
                <div className="option-title">{t.name} {(t.runtime || t.category) && <span className="option-tag">{t.runtime || t.category}</span>}</div>
                <div className="muted">{t.description}</div>
              </div>
            </label>
          ))}
        </div>
      </div>
      {error && <div className="notice notice--error">{error}</div>}
    </Modal>
  )
}
