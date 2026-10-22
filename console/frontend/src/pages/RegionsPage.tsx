import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import { api, listRegions, normNode, relativeTime } from '../api'
import { AddRegionModal } from '../components/AddRegionModal'
import { EmptyState } from '../components/EmptyState'
import { PageHeader } from '../components/PageHeader'
import { StatusBadge } from '../components/StatusBadge'
import { useFetch } from '../hooks/useFetch'
import type { NodeHealth, Project, Region } from '../types'

interface FleetDeployment { project_id: string; image_tag: string; status: string; region: string }
interface FleetRegion { name: string; status: string; deployments: FleetDeployment[] }

interface RegionView {
  region: Region
  nodes: NodeHealth[]
  deployments: FleetDeployment[]
}

async function load(): Promise<{ views: RegionView[]; names: Record<string, string> }> {
  const [regions, fleet, projects] = await Promise.all([
    listRegions(),
    api<{ regions: FleetRegion[] }>('/fleet').catch(() => ({ regions: [] as FleetRegion[] })),
    api<{ projects: Project[] }>('/projects').catch(() => ({ projects: [] as Project[] })),
  ])
  const views = await Promise.all(
    regions.map(async (region): Promise<RegionView> => {
      const nodes = await api<{ nodes: Array<Record<string, unknown>> }>(`/regions/${encodeURIComponent(region.name)}/health`)
        .then((r) => (r.nodes ?? []).map(normNode))
        .catch(() => [] as NodeHealth[])
      const deployments = fleet.regions?.find((f) => f.name === region.name)?.deployments ?? []
      return { region, nodes, deployments }
    }),
  )
  const names: Record<string, string> = {}
  for (const p of projects.projects ?? []) names[p.id] = p.name
  return { views, names }
}

export function RegionsPage() {
  const res = useFetch(load, [], 10000)
  const [adding, setAdding] = useState(false)
  const views = res.data?.views ?? []

  return (
    <div className="page-container">
      <PageHeader title="Regions" action={<button className="btn btn-primary" onClick={() => setAdding(true)}>+ Add region</button>} />
      <p className="muted" style={{ marginBottom: 16 }}>
        A region is a machine, or a small cluster, that runs your apps. Add one, run the command on it, and it appears here.
      </p>

      {res.error && <div className="notice notice--error">{res.error}</div>}

      {res.loading && !res.data && <div className="grid-2">{[1, 2].map((i) => <div key={i} className="skeleton" style={{ height: 150 }} />)}</div>}

      {!res.loading && views.length === 0 && !res.error && (
        <div className="card">
          <EmptyState icon="◎" title="No regions yet" hint="Add a region to get a place to deploy your apps." />
        </div>
      )}

      <div className="grid-2">
        {views.map(({ region, nodes, deployments }) => (
          <div key={region.name} className="card region-card">
            <div className="region-head">
              <span className="region-name">{region.name}</span>
              <StatusBadge status={region.status} size="sm" />
            </div>
            <div className="region-meta muted">
              {nodes.length === 0
                ? region.status === 'pending' ? 'Waiting for a machine to join' : 'No node reports yet'
                : `${nodes.filter((n) => n.ready).length}/${nodes.length} node${nodes.length === 1 ? '' : 's'} ready`}
              {region.created_at && ` · added ${relativeTime(region.created_at)}`}
            </div>
            {nodes.map((n) => (
              <div key={n.name} className="tool-card-row">
                <span>{n.name}</span>
                <span className={n.ready ? 'ok-text' : 'bad-text'}>
                  {n.ready ? 'ready' : 'not ready'}
                  {n.last_heartbeat && <span className="muted"> · {relativeTime(n.last_heartbeat)}</span>}
                </span>
              </div>
            ))}
            <div className="form-label" style={{ marginTop: 12 }}>Apps running</div>
            {deployments.length === 0 ? (
              <div className="muted">Nothing deployed here yet.</div>
            ) : (
              deployments.map((d) => {
                const name = res.data?.names[d.project_id]
                return (
                  <div key={d.project_id + d.image_tag} className="tool-card-row">
                    <span>{name ? <NavLink to={`/apps/${name}`}>{name}</NavLink> : d.project_id.slice(0, 8)}</span>
                    <StatusBadge status={d.status} size="sm" />
                  </div>
                )
              })
            )}
          </div>
        ))}
      </div>

      {adding && <AddRegionModal onClose={() => setAdding(false)} onAdded={() => void res.reload()} />}
    </div>
  )
}
