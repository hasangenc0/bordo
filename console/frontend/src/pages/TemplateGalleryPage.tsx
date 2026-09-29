import { useState, useEffect, useMemo } from 'react'
import type { Template } from '../types'
import { useAuth } from '../hooks/useAuth'
import { Modal } from '../components/Modal'
import { PageHeader } from '../components/PageHeader'

const MOCK_TEMPLATES: Template[] = [
  {
    name: 'java-web-service',
    description: 'Opinionated Spring Boot web service',
    category: 'Backend',
    icon: '☕',
    author: 'Bordo Official',
    deploy_count: 12,
    variables: [],
  },
  {
    name: 'ts-react-app',
    description: 'React + TypeScript frontend app',
    category: 'Frontend',
    icon: '⚛',
    author: 'Bordo Official',
    deploy_count: 8,
    variables: [],
  },
  {
    name: 'node-bff',
    description: 'Node.js Backend-for-Frontend proxy',
    category: 'Backend',
    icon: '⬡',
    author: 'Bordo Official',
    deploy_count: 5,
    variables: [],
  },
  {
    name: 'python-fastapi',
    description: 'FastAPI Python microservice',
    category: 'Backend',
    icon: '🐍',
    author: 'Bordo Official',
    deploy_count: 7,
    variables: [],
  },
  {
    name: 'llm-agent',
    description: 'LLM-powered autonomous agent',
    category: 'AI Agent',
    icon: '🤖',
    author: 'Bordo Official',
    deploy_count: 3,
    variables: [],
  },
  {
    name: 'n8n-automation',
    description: 'n8n workflow automation runner',
    category: 'Automation',
    icon: '⚡',
    author: 'Bordo Official',
    deploy_count: 4,
    variables: [],
  },
]

const CATEGORIES = ['All', 'Backend', 'Frontend', 'AI Agent', 'LLM', 'Automation', 'Data']

type SortMode = 'Most used' | 'Newest' | 'A-Z'

interface DeployModalProps {
  template: Template
  onDeploy: (region: string, vars: Record<string, string>) => void
  onClose: () => void
}

function DeployModal({ template, onDeploy, onClose }: DeployModalProps) {
  const { token } = useAuth()
  const [region, setRegion] = useState('us-east-1')
  const [regions, setRegions] = useState<string[]>(['us-east-1', 'eu-west-1', 'ap-southeast-1'])
  const [varValues, setVarValues] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {}
    for (const v of template.variables) init[v.name] = v.default_val ?? ''
    return init
  })

  useEffect(() => {
    const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
    fetch('/api/v1/regions', { headers })
      .then((r) => r.ok ? r.json() as Promise<{ regions: string[] }> : Promise.reject())
      .then((d) => { if (d.regions?.length) setRegions(d.regions) })
      .catch(() => {})
  }, [token])

  return (
    <Modal
      title={`Deploy ${template.name}`}
      onClose={onClose}
      footer={
        <>
          <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={() => onDeploy(region, varValues)}>
            Deploy
          </button>
        </>
      }
    >
      <div className="form-group">
        <label className="form-label">Target Region</label>
        <select className="select" value={region} onChange={(e) => setRegion(e.target.value)}>
          {regions.map((r) => (
            <option key={r} value={r}>{r}</option>
          ))}
        </select>
      </div>

      {template.variables.map((v) => (
        <div key={v.name} className="form-group">
          <label className="form-label">
            {v.name}
            {v.secret && <span style={{ color: 'var(--text-muted)', fontWeight: 400, marginLeft: '4px' }}>(secret)</span>}
          </label>
          {v.description && (
            <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '4px' }}>{v.description}</div>
          )}
          <input
            className="input"
            type={v.secret ? 'password' : 'text'}
            value={varValues[v.name] ?? ''}
            onChange={(e) => setVarValues((prev) => ({ ...prev, [v.name]: e.target.value }))}
            placeholder={v.default_val}
          />
        </div>
      ))}
    </Modal>
  )
}

interface Props {
  onSendMessage?: (msg: string) => void
}

export function TemplateGalleryPage({ onSendMessage }: Props) {
  const { token } = useAuth()
  const [templates, setTemplates] = useState<Template[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState('All')
  const [sort, setSort] = useState<SortMode>('Most used')
  const [deploying, setDeploying] = useState<Template | null>(null)

  useEffect(() => {
    const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
    fetch('/api/v1/templates', { headers })
      .then((r) => {
        if (!r.ok) throw new Error()
        return r.json() as Promise<{ templates: Template[] }>
      })
      .then((d) => setTemplates(d.templates ?? MOCK_TEMPLATES))
      .catch(() => setTemplates(MOCK_TEMPLATES))
      .finally(() => setLoading(false))
  }, [token])

  const filtered = useMemo(() => {
    let list = templates
    if (search) {
      const q = search.toLowerCase()
      list = list.filter((t) => t.name.toLowerCase().includes(q) || t.description.toLowerCase().includes(q))
    }
    if (category !== 'All') {
      list = list.filter((t) => t.category === category)
    }
    if (sort === 'Most used') list = [...list].sort((a, b) => b.deploy_count - a.deploy_count)
    else if (sort === 'A-Z') list = [...list].sort((a, b) => a.name.localeCompare(b.name))
    return list
  }, [templates, search, category, sort])

  const handleDeploy = (region: string, _vars: Record<string, string>) => {
    if (!deploying) return
    if (onSendMessage) {
      onSendMessage(`deploy from template ${deploying.name} to region ${region}`)
    }
    setDeploying(null)
  }

  return (
    <div className="page-container">
      <PageHeader title="Templates" />

      <div style={{ display: 'flex', gap: '12px', marginBottom: '16px', flexWrap: 'wrap' }}>
        <input
          className="input"
          style={{ width: '240px' }}
          placeholder="Search templates…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <select
          className="select"
          style={{ width: '140px' }}
          value={sort}
          onChange={(e) => setSort(e.target.value as SortMode)}
        >
          <option>Most used</option>
          <option>Newest</option>
          <option>A-Z</option>
        </select>
      </div>

      <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', marginBottom: '24px' }}>
        {CATEGORIES.map((cat) => (
          <button
            key={cat}
            className={`chip${category === cat ? ' chip--active' : ''}`}
            onClick={() => setCategory(cat)}
          >
            {cat}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="grid-3">
          {[1, 2, 3].map((i) => (
            <div key={i} className="skeleton" style={{ height: '160px', borderRadius: 'var(--radius)' }} />
          ))}
        </div>
      ) : (
        <div className="grid-3">
          {filtered.map((t) => (
            <div key={t.name} className="template-card" onClick={() => setDeploying(t)}>
              <span className="template-card-icon">{t.icon}</span>
              <div className="template-card-header">
                <span className="template-card-name">{t.name}</span>
                <span className="template-card-category">{t.category}</span>
              </div>
              <p className="template-card-desc">{t.description}</p>
              <div className="template-card-footer">
                <span>{t.author}</span>
                <span>⊞ {t.deploy_count}</span>
              </div>
            </div>
          ))}
          {filtered.length === 0 && (
            <div style={{ gridColumn: '1/-1', color: 'var(--text-muted)', fontSize: '13px', textAlign: 'center', padding: '40px' }}>
              No templates match your search.
            </div>
          )}
        </div>
      )}

      {deploying && (
        <DeployModal
          template={deploying}
          onDeploy={handleDeploy}
          onClose={() => setDeploying(null)}
        />
      )}
    </div>
  )
}
