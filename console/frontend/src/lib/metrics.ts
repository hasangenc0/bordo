import { api } from '../api'
import type { Series } from '../components/LineChart'

export interface MetricResult {
  series: Series[]
  /** Set when the observability backends aren't wired up. */
  unavailable?: string
  /** Per-region backend errors. */
  errors: string[]
}

export interface RangeOption {
  label: string
  seconds: number
  step: number
}

export const RANGES: RangeOption[] = [
  { label: '1h', seconds: 3600, step: 30 },
  { label: '6h', seconds: 6 * 3600, step: 120 },
  { label: '24h', seconds: 24 * 3600, step: 300 },
  { label: '3d', seconds: 3 * 24 * 3600, step: 900 },
]

type VMResponse = {
  status?: string
  error?: string
  data?: { result?: Array<{ metric?: Record<string, string>; values?: Array<[number, string]> }> }
}

function labelFor(region: string, metric: Record<string, string> | undefined, multiRegion: boolean, multiSeries: boolean): string {
  const rest = Object.entries(metric ?? {}).filter(([k]) => k !== '__name__' && k !== 'job' && k !== 'instance')
  const detail = rest.map(([k, v]) => `${k}=${v}`).join(' ')
  const parts = [multiRegion ? region : '', multiSeries ? detail : ''].filter(Boolean)
  return parts.join(' · ') || region
}

/** Runs a PromQL range query across all regions (or one) via the control-plane observe facade. */
export async function queryRange(promql: string, range: RangeOption, region?: string, signal?: AbortSignal): Promise<MetricResult> {
  const end = Math.floor(Date.now() / 1000)
  const qs = new URLSearchParams({
    query: promql,
    start: String(end - range.seconds),
    end: String(end),
    step: `${range.step}s`,
  })
  if (region) qs.set('region', region)

  const raw = await api<Record<string, unknown>>(`/observe/metrics?${qs}`, { signal })
  if (raw && typeof raw.status === 'string' && raw.status === 'backends_not_configured') {
    return { series: [], unavailable: String(raw.message ?? 'Observability backends are not configured'), errors: [] }
  }

  const regions = Object.entries(raw ?? {}) as Array<[string, VMResponse]>
  const multiRegion = regions.length > 1
  const series: Series[] = []
  const errors: string[] = []
  for (const [name, resp] of regions) {
    if (!resp || typeof resp !== 'object') continue
    if (resp.error) {
      errors.push(`${name}: ${resp.error}`)
      continue
    }
    const result = resp.data?.result ?? []
    for (const r of result) {
      const points: Array<[number, number]> = (r.values ?? [])
        .map(([t, v]): [number, number] => [Number(t), Number(v)])
        .filter(([, v]) => Number.isFinite(v))
      if (points.length) series.push({ label: labelFor(name, r.metric, multiRegion, result.length > 1), points })
    }
  }
  return { series, errors }
}

export interface LogLine {
  ts: number // ms
  region: string
  line: string
}

type LokiResponse = {
  error?: string
  data?: { result?: Array<{ stream?: Record<string, string>; values?: Array<[string, string]> }> }
}

export async function queryLogs(logql: string, rangeSeconds: number, region?: string, signal?: AbortSignal): Promise<{ lines: LogLine[]; unavailable?: string; errors: string[] }> {
  const endNs = BigInt(Date.now()) * 1_000_000n
  const startNs = endNs - BigInt(rangeSeconds) * 1_000_000_000n
  const qs = new URLSearchParams({ query: logql, start: startNs.toString(), end: endNs.toString() })
  if (region) qs.set('region', region)
  const raw = await api<Record<string, unknown>>(`/observe/logs?${qs}`, { signal })
  if (raw && typeof raw.status === 'string' && raw.status === 'backends_not_configured') {
    return { lines: [], unavailable: String(raw.message ?? 'Observability backends are not configured'), errors: [] }
  }
  const lines: LogLine[] = []
  const errors: string[] = []
  for (const [name, resp] of Object.entries(raw ?? {}) as Array<[string, LokiResponse]>) {
    if (!resp || typeof resp !== 'object') continue
    if (resp.error) {
      errors.push(`${name}: ${resp.error}`)
      continue
    }
    for (const stream of resp.data?.result ?? []) {
      for (const [ns, line] of stream.values ?? []) {
        lines.push({ ts: Number(BigInt(ns) / 1_000_000n), region: name, line })
      }
    }
  }
  lines.sort((a, b) => b.ts - a.ts)
  return { lines, errors }
}

// OTLP → remote-write sets `job` from service.name (optionally "<namespace>/<name>").
// Centralised so there's exactly one place to change if the collector config adds
// resource-to-label conversion.
export const appSelector = (name: string) => `job=~"(.*/)?${name.replace(/[^a-zA-Z0-9_-]/g, '')}"`
export const appLogSelector = (name: string) => `{job=~"(.*/)?${name.replace(/[^a-zA-Z0-9_-]/g, '')}"}`

export interface Preset {
  id: string
  title: string
  query: (app: string) => string
  format: (v: number) => string
}

const fmtNum = (v: number) => (v >= 100 ? v.toFixed(0) : v >= 1 ? v.toFixed(1) : v.toFixed(2))
const fmtBytes = (v: number) => (v >= 1 << 30 ? `${(v / (1 << 30)).toFixed(1)} GB` : `${Math.round(v / (1 << 20))} MB`)

export const APP_PRESETS: Preset[] = [
  {
    id: 'rps',
    title: 'Requests / sec',
    query: (a) => `sum(rate(http_server_request_duration_seconds_count{${appSelector(a)}}[5m]))`,
    format: fmtNum,
  },
  {
    id: 'errors',
    title: '5xx errors / sec',
    query: (a) => `sum(rate(http_server_request_duration_seconds_count{${appSelector(a)},http_response_status_code=~"5.."}[5m]))`,
    format: fmtNum,
  },
  {
    id: 'p95',
    title: 'p95 latency (s)',
    query: (a) => `histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket{${appSelector(a)}}[5m])))`,
    format: (v) => `${fmtNum(v)}s`,
  },
  {
    id: 'mem',
    title: 'Memory',
    query: (a) => `sum(process_runtime_jvm_memory_usage_bytes{${appSelector(a)}}) or sum(process_resident_memory_bytes{${appSelector(a)}})`,
    format: fmtBytes,
  },
]
