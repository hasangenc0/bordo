import type { Build, NodeHealth, Region } from './types'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

function authHeaders(): Record<string, string> {
  try {
    const t = localStorage.getItem('bordo_token')
    return t ? { Authorization: `Bearer ${t}` } : {}
  } catch {
    return {}
  }
}

/** Calls the control plane through the console proxy: /api/v1/<path>. */
export async function api<T>(path: string, init: { method?: string; json?: unknown; signal?: AbortSignal } = {}): Promise<T> {
  const headers: Record<string, string> = { ...authHeaders() }
  if (init.json !== undefined) headers['Content-Type'] = 'application/json'
  let res: Response
  try {
    res = await fetch(`/api/v1${path}`, {
      method: init.method ?? (init.json !== undefined ? 'POST' : 'GET'),
      headers,
      body: init.json !== undefined ? JSON.stringify(init.json) : undefined,
      signal: init.signal,
    })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') throw e
    throw new ApiError(0, 'Cannot reach the Bordo server')
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  let body: unknown = undefined
  try {
    body = text ? JSON.parse(text) : undefined
  } catch {
    // non-JSON error bodies (e.g. a proxy 502 page)
  }
  if (!res.ok) {
    const msg = (body as { error?: string } | undefined)?.error ?? `Request failed (${res.status})`
    throw new ApiError(res.status, msg)
  }
  return body as T
}

// The control plane serializes some structs without json tags (Go field names),
// so accept either casing instead of assuming one.
type Raw = Record<string, unknown>
const str = (r: Raw, ...keys: string[]): string => {
  for (const k of keys) if (typeof r[k] === 'string') return r[k] as string
  return ''
}

export function normRegion(r: Raw): Region {
  return {
    id: str(r, 'id', 'ID'),
    name: str(r, 'name', 'Name'),
    status: str(r, 'status', 'Status') || 'unknown',
    created_at: str(r, 'created_at', 'CreatedAt'),
  }
}

export function normBuild(r: Raw): Build {
  return {
    id: str(r, 'id', 'ID'),
    project_id: str(r, 'project_id', 'ProjectID'),
    status: str(r, 'status', 'Status') || 'unknown',
    image_name: str(r, 'image_name', 'ImageName'),
    image_tag: str(r, 'image_tag', 'ImageTag'),
    error_message: str(r, 'error_message', 'ErrorMessage'),
    created_at: str(r, 'created_at', 'CreatedAt'),
    completed_at: str(r, 'completed_at', 'CompletedAt') || undefined,
  }
}

export function normNode(r: Raw): NodeHealth {
  return {
    name: str(r, 'name', 'Name'),
    ready: Boolean(r.ready ?? r.Ready),
    last_heartbeat: str(r, 'last_heartbeat', 'LastHeartbeat', 'lastHeartbeat') || undefined,
  }
}

export const listRegions = async (): Promise<Region[]> =>
  ((await api<{ regions: Raw[] }>('/regions')).regions ?? []).map(normRegion)

export const listBuilds = async (projectId?: string): Promise<Build[]> =>
  ((await api<{ builds: Raw[] }>(`/builds${projectId ? `?project_id=${encodeURIComponent(projectId)}` : ''}`)).builds ?? []).map(normBuild)

export function relativeTime(iso: string | undefined): string {
  if (!iso) return '—'
  const t = new Date(iso).getTime()
  if (Number.isNaN(t) || t < 1e11) return '—'
  const diff = Math.max(0, Math.floor((Date.now() - t) / 1000))
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}
