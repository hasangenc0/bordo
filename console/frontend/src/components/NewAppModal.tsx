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
  const [source, setSource] = useState<'template' | 'repo'>('template')
  const [repoUrl, setRepoUrl] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const list = templates.data ?? []
  const chosen = template || list[0]?.name || ''
  const nameOk = NAME_RE.test(name)
  const repoOk = /^(https:\/\/github\.com\/|git@github\.com:)[\w.-]+\/[\w.-]+?(\.git)?\/?$/.test(repoUrl.trim())
  // An imported repo can name the project itself; a template needs a valid name.
  const canSubmit = source === 'template' ? nameOk && !!chosen : repoOk && (name === '' || nameOk)

  const submit = async () => {
    if (!canSubmit) return
    setSubmitting(true)
    setError(null)
    try {
      const body = source === 'repo' ? { name: name || undefined, git_repo_url: repoUrl.trim() } : { name, template: chosen }
      const p = await api<Project>('/projects', { json: body })
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
          <button className="btn btn-primary" onClick={() => void submit()} disabled={!canSubmit || submitting}>
            {submitting ? 'Creating…' : 'Create app'}
          </button>
        </>
      }
    >
      <div className="chip-row" style={{ marginBottom: 14 }} role="tablist" aria-label="Where does the code come from?">
        <button type="button" role="tab" aria-selected={source === 'template'} className={`chip${source === 'template' ? ' chip--active' : ''}`} onClick={() => setSource('template')}>From a template</button>
        <button type="button" role="tab" aria-selected={source === 'repo'} className={`chip${source === 'repo' ? ' chip--active' : ''}`} onClick={() => setSource('repo')}>Existing GitHub repo</button>
      </div>
      {source === 'repo' && (
        <div className="form-group">
          <label className="form-label">Repository</label>
          <input className="input" autoFocus placeholder="https://github.com/you/your-repo" value={repoUrl} onChange={(e) => setRepoUrl(e.target.value)} />
          <div className="muted" style={{ marginTop: 6 }}>
            Bordo builds your repo's latest commit and never changes it without asking. It needs a Dockerfile at the root and the Bordo build workflow; the agent can add the workflow when you agree.
          </div>
        </div>
      )}
      <div className="form-group">
        <label className="form-label">Name{source === 'repo' ? ' (optional)' : ''}</label>
        <input className="input" autoFocus={source === 'template'} placeholder={source === 'repo' ? 'defaults to the repo name' : 'hello-api'} value={name} onChange={(e) => setName(e.target.value.toLowerCase())} />
        {name && !nameOk && <div className="muted" style={{ marginTop: 6 }}>Use 3–40 lowercase letters, numbers and dashes, starting with a letter.</div>}
      </div>
      {source === 'template' && (
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
      )}
      {error && <div className="notice notice--error">{error}</div>}
    </Modal>
  )
}
