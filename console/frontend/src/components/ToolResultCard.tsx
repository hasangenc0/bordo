interface Props {
  toolName: string
  data: unknown
}

function ProjectCard({ data }: { data: Record<string, unknown> }) {
  return (
    <div className="tool-card project-card">
      <div className="tool-card-header">📁 Project</div>
      <div className="tool-card-row"><span>Name</span><strong>{String(data.name ?? '—')}</strong></div>
      <div className="tool-card-row"><span>ID</span><code>{String(data.id ?? '—')}</code></div>
      <div className="tool-card-row"><span>Status</span><span className="badge">{String(data.status ?? 'unknown')}</span></div>
    </div>
  )
}

function BuildCard({ data }: { data: Record<string, unknown> }) {
  return (
    <div className="tool-card build-card">
      <div className="tool-card-header">🔨 Build</div>
      <div className="tool-card-row"><span>ID</span><code>{String(data.id ?? '—')}</code></div>
      <div className="tool-card-row"><span>Status</span><span className="badge">{String(data.status ?? 'unknown')}</span></div>
      {data.image_tag && (
        <div className="tool-card-row"><span>Image</span><code>{String(data.image_tag)}</code></div>
      )}
    </div>
  )
}

function DeployCard({ data }: { data: Record<string, unknown> }) {
  return (
    <div className="tool-card deploy-card">
      <div className="tool-card-header">🚀 Deploy</div>
      <div className="tool-card-row"><span>ID</span><code>{String(data.id ?? '—')}</code></div>
      <div className="tool-card-row"><span>Region</span><strong>{String(data.region ?? '—')}</strong></div>
      <div className="tool-card-row"><span>Status</span><span className="badge">{String(data.status ?? 'unknown')}</span></div>
    </div>
  )
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

export function ToolResultCard({ toolName, data }: Props) {
  if (isRecord(data)) {
    if (toolName.startsWith('project_')) return <ProjectCard data={data} />
    if (toolName.startsWith('build_')) return <BuildCard data={data} />
    if (toolName.startsWith('deploy')) return <DeployCard data={data} />
  }

  return (
    <div className="tool-card fallback-card">
      <div className="tool-card-header">🔧 {toolName}</div>
      <pre className="tool-card-json">{JSON.stringify(data, null, 2)}</pre>
    </div>
  )
}
