import { useEffect, useRef, useState } from 'react'
import { listRegions } from '../api'
import { LineChart } from '../components/LineChart'
import { PageHeader } from '../components/PageHeader'
import { useFetch } from '../hooks/useFetch'
import { queryLogs, queryRange, RANGES, type LogLine, type MetricResult } from '../lib/metrics'

type Mode = 'metrics' | 'logs'

const METRIC_EXAMPLES = ['up', 'sum(rate(http_server_request_duration_seconds_count[5m]))', 'sum by (job) (rate(http_server_request_duration_seconds_count[5m]))']
const LOG_EXAMPLES = ['{job=~".+"}', '{job=~".+"} |= "error"']

export function ObservePage() {
  const [mode, setMode] = useState<Mode>('metrics')
  const [query, setQuery] = useState(METRIC_EXAMPLES[0])
  const [range, setRange] = useState(RANGES[0])
  const [region, setRegion] = useState('')
  const regions = useFetch(listRegions, [])

  const [metric, setMetric] = useState<MetricResult | null>(null)
  const [logs, setLogs] = useState<{ lines: LogLine[]; unavailable?: string; errors: string[] } | null>(null)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const run = async (m: Mode = mode, q: string = query) => {
    abortRef.current?.abort()
    const ac = new AbortController()
    abortRef.current = ac
    setRunning(true)
    setError(null)
    try {
      if (m === 'metrics') setMetric(await queryRange(q, range, region || undefined, ac.signal))
      else setLogs(await queryLogs(q, range.seconds, region || undefined, ac.signal))
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return
      setError(e instanceof Error ? e.message : 'Query failed')
    } finally {
      if (abortRef.current === ac) setRunning(false)
    }
  }

  useEffect(() => () => abortRef.current?.abort(), [])

  const switchMode = (m: Mode) => {
    setMode(m)
    setQuery(m === 'metrics' ? METRIC_EXAMPLES[0] : LOG_EXAMPLES[0])
    setError(null)
  }

  const examples = mode === 'metrics' ? METRIC_EXAMPLES : LOG_EXAMPLES
  const unavailable = mode === 'metrics' ? metric?.unavailable : logs?.unavailable
  const errors = (mode === 'metrics' ? metric?.errors : logs?.errors) ?? []

  return (
    <div className="page-container">
      <PageHeader title="Observe" />
      <div className="tabs">
        <button className={`tab${mode === 'metrics' ? ' tab--active' : ''}`} onClick={() => switchMode('metrics')}>Metrics</button>
        <button className={`tab${mode === 'logs' ? ' tab--active' : ''}`} onClick={() => switchMode('logs')}>Logs</button>
      </div>

      <div className="query-box">
        <textarea
          className="input mono"
          rows={2}
          value={query}
          spellCheck={false}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) void run()
          }}
          placeholder={mode === 'metrics' ? 'PromQL' : 'LogQL'}
        />
        <div className="toolbar">
          <div className="time-range-group">
            {RANGES.map((r) => (
              <button key={r.label} className={`time-range-btn${range.label === r.label ? ' time-range-btn--active' : ''}`} onClick={() => setRange(r)}>{r.label}</button>
            ))}
          </div>
          <select className="select" style={{ width: 160 }} value={region} onChange={(e) => setRegion(e.target.value)}>
            <option value="">All regions</option>
            {(regions.data ?? []).map((r) => <option key={r.name} value={r.name}>{r.name}</option>)}
          </select>
          <button className="btn btn-primary" onClick={() => void run()} disabled={running || !query.trim()}>{running ? 'Running…' : 'Run'}</button>
          <span className="muted">⌘/Ctrl + Enter</span>
        </div>
        <div className="chip-row">
          {examples.map((q) => <button key={q} className="chip mono" onClick={() => { setQuery(q); void run(mode, q) }}>{q}</button>)}
        </div>
      </div>

      {error && <div className="notice notice--error">{error}</div>}
      {unavailable && (
        <div className="notice">
          Observability isn't set up on this server. Deploy the observability stack and set each region's backend URLs.
          <div className="muted" style={{ marginTop: 4 }}>{unavailable}</div>
        </div>
      )}
      {errors.map((e) => <div key={e} className="notice notice--error">{e}</div>)}

      {mode === 'metrics' && metric && !unavailable && (
        <div className="stat-card"><LineChart series={metric.series} height={240} /></div>
      )}
      {mode === 'logs' && logs && !unavailable && (
        logs.lines.length === 0 ? <div className="muted">No log lines matched.</div> : (
          <div className="log-block">
            {logs.lines.map((l, i) => (
              <div key={i}><span className="log-ts">{new Date(l.ts).toLocaleTimeString()}</span> {regions.data && regions.data.length > 1 && <span className="log-region">[{l.region}] </span>}{l.line}</div>
            ))}
          </div>
        )
      )}
    </div>
  )
}
