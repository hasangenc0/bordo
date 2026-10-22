import { useState, type ReactNode } from 'react'
import type { ToolActivity } from '../types'
import { CopyButton } from './CopyButton'
import { DiffView } from './DiffView'
import { StatusBadge } from './StatusBadge'

type Rec = Record<string, unknown>
const isRec = (v: unknown): v is Rec => typeof v === 'object' && v !== null && !Array.isArray(v)
const s = (v: unknown): string => (typeof v === 'string' ? v : v == null ? '' : String(v))

const LABELS: Record<string, string> = {
  code_checkout: 'Checked out code',
  code_list_files: 'Listed files',
  code_read_file: 'Read',
  code_write_file: 'Wrote',
  code_edit_file: 'Edited',
  code_search: 'Searched code',
  code_diff: 'Reviewed changes',
  code_commit_push: 'Pushed branch',
  code_open_pr: 'Opened pull request',
  project_create: 'Created project',
  project_list: 'Listed projects',
  build_trigger: 'Started build',
  build_status: 'Checked build',
  build_logs: 'Fetched build logs',
  deploy: 'Deployed',
  deploy_status: 'Checked release',
  deploy_logs: 'Fetched deploy logs',
  rollback: 'Rolled back',
  region_list: 'Listed regions',
  region_add: 'Added region',
  region_health: 'Checked region health',
  fleet_overview: 'Fleet overview',
  metrics_query: 'Queried metrics',
  logs_query: 'Queried logs',
}

const RUNNING_LABELS: Record<string, string> = {
  code_checkout: 'Checking out code',
  code_read_file: 'Reading',
  code_write_file: 'Writing',
  code_edit_file: 'Editing',
  code_commit_push: 'Pushing branch',
  deploy: 'Deploying',
  build_trigger: 'Starting build',
}

function title(t: ToolActivity): string {
  const target = s(t.input?.path) || s(t.input?.query) || s(t.input?.name) || s(t.input?.region)
  const base =
    t.status === 'running'
      ? (RUNNING_LABELS[t.name] ?? (LABELS[t.name] ?? t.name.replace(/_/g, ' ')) + '…')
      : (LABELS[t.name] ?? t.name.replace(/_/g, ' '))
  const full = target ? `${base} ${target}` : base
  return t.status === 'error' ? `${full} — failed` : full
}

function Rows({ rows }: { rows: Array<[string, ReactNode]> }) {
  return (
    <div className="tc-rows">
      {rows.filter(([, v]) => v !== '' && v != null).map(([k, v]) => (
        <div key={k} className="tool-card-row"><span>{k}</span><span>{v}</span></div>
      ))}
    </div>
  )
}

function Rich({ t }: { t: ToolActivity }): ReactNode {
  const d = t.data
  if (!isRec(d)) return null

  switch (t.name) {
    case 'code_diff': {
      const files = Array.isArray(d.files) ? d.files.length : 0
      return (
        <>
          <Rows rows={[['Branch', <code key="b">{s(d.branch)}</code>], ['Files changed', files ? String(files) : '']]} />
          <DiffView diff={s(d.diff)} truncated={d.truncated === true} />
        </>
      )
    }
    case 'code_checkout':
      return <Rows rows={[['Branch', <code key="b">{s(d.branch)}</code>], ['Based on', <code key="c">{s(d.base)}</code>]]} />
    case 'code_commit_push':
      return (
        <Rows
          rows={[
            ['Branch', <code key="b">{s(d.branch)}</code>],
            ['Commit', <code key="c">{s(d.commit).slice(0, 8)}</code>],
            ['Compare', d.compare_url ? <a key="u" href={s(d.compare_url)} target="_blank" rel="noreferrer noopener">View on GitHub ↗</a> : ''],
          ]}
        />
      )
    case 'code_open_pr':
      return (
        <Rows
          rows={[
            ['Pull request', d.url ? <a key="u" href={s(d.url)} target="_blank" rel="noreferrer noopener">#{s(d.number)} on GitHub ↗</a> : ''],
          ]}
        />
      )
    case 'region_add': {
      const cmd = s(d.join_command)
      return (
        <>
          <Rows rows={[['Region', <strong key="r">{s(d.region)}</strong>], ['Expires', s(d.expires_at)]]} />
          {cmd && (
            <div className="md-fence md-fence--wrap">
              <div className="md-fence-bar"><span>Run this on the target machine</span><CopyButton text={cmd} /></div>
              <pre>{cmd}</pre>
            </div>
          )}
        </>
      )
    }
    case 'deploy':
    case 'deploy_status':
    case 'rollback': {
      const releases = Array.isArray(d.releases) ? (d.releases as Rec[]) : [d]
      return (
        <div className="tc-list">
          {releases.map((r, i) => (
            <div key={s(r.id) || i} className="tool-card-row">
              <span>{s(r.region) || 'release'}</span>
              <span><StatusBadge status={s(r.status) || 'unknown'} size="sm" /></span>
            </div>
          ))}
        </div>
      )
    }
    case 'build_trigger':
    case 'build_status': {
      const status = s(d.status) || s(d.Status)
      return <Rows rows={[['Status', status ? <StatusBadge key="s" status={status} size="sm" /> : ''], ['Image', <code key="i">{s(d.image_tag ?? d.ImageTag)}</code>]]} />
    }
    default:
      return null
  }
}

const ALWAYS_OPEN = new Set(['code_diff', 'code_commit_push', 'code_open_pr', 'region_add', 'deploy', 'rollback', 'build_trigger'])

export function ToolCard({ tool }: { tool: ToolActivity }) {
  const [open, setOpen] = useState(false)
  const showRich = tool.status === 'done' && ALWAYS_OPEN.has(tool.name)
  const hasDetail = tool.data !== undefined || tool.input !== undefined || tool.error

  return (
    <div className={`tool-activity tool-activity--${tool.status}`}>
      <button type="button" className="tool-activity-head" onClick={() => hasDetail && setOpen((o) => !o)} aria-expanded={open}>
        {tool.status === 'running' ? <span className="spinner" aria-label="running" /> : (
          <span className="tool-activity-icon">{tool.status === 'error' ? '✕' : '✓'}</span>
        )}
        <span className="tool-activity-title">{title(tool)}</span>
        {hasDetail && <span className="tool-activity-caret">{open ? '▾' : '▸'}</span>}
      </button>
      {tool.status === 'error' && tool.error && <div className="tool-activity-error">{tool.error}</div>}
      {showRich && <div className="tool-activity-rich"><Rich t={tool} /></div>}
      {open && (
        <div className="tool-activity-detail">
          {tool.input && Object.keys(tool.input).length > 0 && <pre className="tool-card-json">{JSON.stringify(tool.input, null, 2)}</pre>}
          {tool.data !== undefined && <pre className="tool-card-json">{JSON.stringify(tool.data, null, 2)}</pre>}
        </div>
      )}
    </div>
  )
}
