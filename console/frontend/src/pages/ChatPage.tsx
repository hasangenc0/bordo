import { NavLink } from 'react-router-dom'
import { api, listRegions } from '../api'
import { ChatPane } from '../components/ChatPane'
import { StatusBadge } from '../components/StatusBadge'
import { useFetch } from '../hooks/useFetch'
import type { ChatSession, Message, Project } from '../types'
import { ArrowRight, X } from 'lucide-react'

interface Props {
  chats: ChatSession[]
  activeChatId: string | null
  onSelect: (id: string) => void
  onNew: () => void
  onDelete: (id: string) => void
  messages: Message[]
  onSend: (text: string) => void
  onStop: () => void
  connected: boolean
  busy: boolean
  title?: string
}

function FleetRail() {
  const apps = useFetch(async () => (await api<{ projects: Project[] }>('/projects?include=release_status')).projects ?? [], [], 15000)
  const regions = useFetch(listRegions, [], 15000)
  return (
    <aside className="rail rail--right">
      <div className="rail-section">
        <div className="rail-title">Apps</div>
        {(apps.data ?? []).length === 0 && <div className="muted">None yet</div>}
        {(apps.data ?? []).slice(0, 8).map((p) => (
          <NavLink key={p.id} to={`/apps/${p.name}`} className="rail-row">
            <span className="ellipsis">{p.name}</span>
            <StatusBadge status={p.release_status || 'unknown'} size="sm" />
          </NavLink>
        ))}
        <NavLink to="/apps" className="rail-link">All apps <ArrowRight size={12} className="icon-inline" aria-hidden="true" /></NavLink>
      </div>
      <div className="rail-section">
        <div className="rail-title">Regions</div>
        {(regions.data ?? []).length === 0 && <div className="muted">None yet</div>}
        {(regions.data ?? []).map((r) => (
          <div key={r.name} className="rail-row">
            <span className="ellipsis">{r.name}</span>
            <StatusBadge status={r.status} size="sm" />
          </div>
        ))}
        <NavLink to="/regions" className="rail-link">Manage regions <ArrowRight size={12} className="icon-inline" aria-hidden="true" /></NavLink>
      </div>
    </aside>
  )
}

export function ChatPage({ chats, activeChatId, onSelect, onNew, onDelete, messages, onSend, onStop, connected, busy, title }: Props) {
  return (
    <div className="chat-page">
      <aside className="rail rail--left">
        <button className="btn btn-primary" style={{ width: '100%' }} onClick={onNew}>+ New chat</button>
        <div className="rail-title" style={{ marginTop: 16 }}>History</div>
        <div className="chat-list">
          {chats.length === 0 && <div className="muted">No conversations yet</div>}
          {chats.map((c) => (
            <div key={c.id} className={`chat-list-item${c.id === activeChatId ? ' chat-list-item--active' : ''}`}>
              <button type="button" className="chat-list-title" onClick={() => onSelect(c.id)} title={c.title}>
                {c.title || 'New chat'}
              </button>
              <button
                type="button"
                className="chat-list-del"
                aria-label={`Delete ${c.title}`}
                onClick={() => { if (window.confirm('Delete this conversation?')) onDelete(c.id) }}
              >
                <X size={12} aria-hidden="true" />
              </button>
            </div>
          ))}
        </div>
      </aside>
      <section className="chat-main">
        <ChatPane
          variant="page"
          messages={messages}
          onSend={onSend}
          onStop={onStop}
          connected={connected}
          busy={busy}
          chatId={activeChatId}
          title={title}
        />
      </section>
      <FleetRail />
    </div>
  )
}
