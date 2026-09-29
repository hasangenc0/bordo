import { useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import type { ActionEvent, WSIncoming } from './types'
import { Sidebar } from './components/Sidebar'
import { ChatPane } from './components/ChatPane'
import { ActivityPanel } from './components/ActivityPanel'
import { ServicesPage } from './pages/ServicesPage'
import { ServiceDetailPage } from './pages/ServiceDetailPage'
import { TemplateGalleryPage } from './pages/TemplateGalleryPage'
import { useAuth } from './hooks/useAuth'
import { useWebSocket } from './hooks/useWebSocket'
import { useChats } from './hooks/useChats'
import './styles.css'

const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
const AGENT_WS_URL =
  (import.meta.env.VITE_AGENT_WS_URL as string | undefined) ??
  `${wsProto}//${window.location.host}/ws/chat`

function SettingsPanel({ token, onSave }: { token: string; onSave: (t: string) => void }) {
  const [draft, setDraft] = useState(token)
  return (
    <div className="settings-panel">
      <h2>Settings</h2>
      <label className="settings-label">
        Bearer Token
        <input
          type="password"
          className="settings-input"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Enter your Bordo API token"
        />
      </label>
      <button className="settings-save" onClick={() => onSave(draft)}>
        Save
      </button>
      <p className="settings-hint">
        Token is stored in localStorage. Generate one with <code>bordo token create</code>.
      </p>
    </div>
  )
}

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className="page-container">
      <h1 className="page-title">{title}</h1>
      <p style={{ color: 'var(--text-muted)', marginTop: '16px', fontSize: '13px' }}>
        This section is coming soon.
      </p>
    </div>
  )
}

export default function App() {
  const { token, saveToken } = useAuth()
  const [activeChatId, setActiveChatId] = useState<string | null>(null)
  const [showActivity, setShowActivity] = useState(false)
  const [actionEvents, setActionEvents] = useState<ActionEvent[]>([])
  const { chats, createChat, updateChatTitle } = useChats()

  const activeChat = chats.find((c) => c.id === activeChatId)
  const { messages, send, connected } = useWebSocket(AGENT_WS_URL, token, activeChatId, (msg: WSIncoming) => {
    if (msg.type === 'action' && msg.action_id) {
      const ev: ActionEvent = {
        id: msg.action_id,
        kind: msg.action_kind ?? 'action',
        target: msg.action_target ?? '',
        status: (msg.action_status as ActionEvent['status']) ?? 'auto_approved',
        ts: msg.action_ts ?? Date.now(),
        log_url: msg.action_log_url,
      }
      setActionEvents((prev) => {
        const existing = prev.findIndex((e) => e.id === ev.id)
        if (existing >= 0) {
          const updated = [...prev]
          updated[existing] = ev
          return updated
        }
        return [ev, ...prev]
      })
      setShowActivity(true)
    }
  })

  const handleNewChat = async () => {
    const session = await createChat()
    if (session) {
      setActiveChatId(session.id)
    }
  }

  const handleChatSelect = (id: string) => {
    setActiveChatId(id)
  }

  const handleSendMessage = (msg: string) => {
    if (!activeChatId) {
      createChat().then((session) => {
        if (session) {
          setActiveChatId(session.id)
          // Send after connection is established — the WS hook will handle it
          setTimeout(() => send(msg), 500)
          if (session.title === 'New Chat') {
            updateChatTitle(session.id, msg.slice(0, 40))
          }
        }
      })
    } else {
      send(msg)
    }
  }

  const approveAction = async (id: string) => {
    await fetch(`/api/approvals/${id}/approve`, { method: 'POST' }).catch(() => {})
    setActionEvents((prev) =>
      prev.map((e) => e.id === id ? { ...e, status: 'approved' } : e),
    )
  }

  const rejectAction = async (id: string) => {
    await fetch(`/api/approvals/${id}/reject`, { method: 'POST' }).catch(() => {})
    setActionEvents((prev) =>
      prev.map((e) => e.id === id ? { ...e, status: 'failed' } : e),
    )
  }

  return (
    <BrowserRouter>
      <div className="app-layout">
        <Sidebar />
        <main className="app-main">
          <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
            <div className="app-content" style={{ flex: 1, overflow: 'auto' }}>
              <Routes>
                <Route path="/" element={<Navigate to="/services" replace />} />
                <Route
                  path="/services"
                  element={<ServicesPage onSendMessage={handleSendMessage} />}
                />
                <Route
                  path="/services/:name/*"
                  element={<ServiceDetailPage onSendMessage={handleSendMessage} />}
                />
                <Route
                  path="/templates"
                  element={<TemplateGalleryPage onSendMessage={handleSendMessage} />}
                />
                <Route path="/build" element={<PlaceholderPage title="Build" />} />
                <Route path="/release" element={<PlaceholderPage title="Release" />} />
                <Route path="/observe" element={<PlaceholderPage title="Observe" />} />
                <Route
                  path="/settings"
                  element={
                    <div className="app-content">
                      <SettingsPanel token={token} onSave={saveToken} />
                    </div>
                  }
                />
              </Routes>
            </div>

            {/* Chat pane */}
            <div style={{ width: '360px', borderLeft: '1px solid var(--border)', display: 'flex', flexDirection: 'column', flexShrink: 0 }}>
              <div style={{ padding: '8px 16px', borderBottom: '1px solid var(--border)', display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' }}>
                <span style={{ fontSize: '12px', fontWeight: 500, flex: 1 }}>Agent Chat</span>
                <button className="btn btn-secondary btn-sm" onClick={handleNewChat}>+ New Chat</button>
                {chats.slice(0, 3).map((c) => (
                  <button
                    key={c.id}
                    className={`btn btn-sm ${activeChatId === c.id ? 'btn-primary' : 'btn-secondary'}`}
                    onClick={() => handleChatSelect(c.id)}
                    style={{ maxWidth: '100px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                    title={c.title}
                  >
                    {c.title.slice(0, 12)}
                  </button>
                ))}
                <button
                  className="activity-toggle"
                  style={{ position: 'static', marginLeft: 'auto' }}
                  onClick={() => setShowActivity((v) => !v)}
                >
                  Activity {actionEvents.filter((e) => e.status === 'pending').length > 0 && (
                    <span style={{ color: 'var(--status-warning)' }}>●</span>
                  )}
                </button>
              </div>
              <ChatPane
                messages={messages}
                onSend={send}
                connected={connected}
                chatId={activeChatId}
                title={activeChat?.title}
              />
            </div>

            {showActivity && (
              <ActivityPanel
                events={actionEvents}
                onApprove={approveAction}
                onReject={rejectAction}
                onClose={() => setShowActivity(false)}
              />
            )}
          </div>
        </main>
      </div>
    </BrowserRouter>
  )
}
