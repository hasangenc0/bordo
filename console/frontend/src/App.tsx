import { useCallback, useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import type { ActionEvent, WSIncoming } from './types'
import { ActivityPanel } from './components/ActivityPanel'
import { ChatPane } from './components/ChatPane'
import { Logo } from './components/Logo'
import { SetupGate } from './components/SetupGate'
import { GithubConnect } from './components/GithubConnect'
import { Sidebar } from './components/Sidebar'
import { api } from './api'
import { useAuth } from './hooks/useAuth'
import { useFetch } from './hooks/useFetch'
import { useChats } from './hooks/useChats'
import { useWebSocket } from './hooks/useWebSocket'
import { AppDetailPage } from './pages/AppDetailPage'
import { AppsPage } from './pages/AppsPage'
import { ChatPage } from './pages/ChatPage'
import { ObservePage } from './pages/ObservePage'
import { RegionsPage } from './pages/RegionsPage'
import './styles.css'
import './theme.css'
import { ListChecks, Plus, Sparkles, X } from 'lucide-react'

const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
const AGENT_WS_URL =
  (import.meta.env.VITE_AGENT_WS_URL as string | undefined) ??
  `${wsProto}//${window.location.host}/ws/chat`

const ACTIVE_CHAT_KEY = 'bordo_active_chat'
const DOCK_KEY = 'bordo_dock_open'

function readStore(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
function writeStore(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key)
    else localStorage.setItem(key, value)
  } catch {
    // storage unavailable (private mode); the setting just won't persist
  }
}

function SettingsPanel({ token, onSave }: { token: string; onSave: (t: string) => void }) {
  const [draft, setDraft] = useState(token)
  const { data: setup, reload } = useFetch(() => api<{ github_connected: boolean; github_device_ready: boolean }>('/setup/status'), [])
  return (
    <div className="settings-panel">
      <h2>Settings</h2>
      {setup && (
        <section className="settings-section">
          <h3>GitHub</h3>
          <GithubConnect deviceReady={setup.github_device_ready} connected={setup.github_connected} onConnected={() => void reload()} />
        </section>
      )}
      <label className="settings-label">
        Bearer token
        <input
          type="password"
          className="settings-input"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Only needed when the console is not behind the Bordo proxy"
        />
      </label>
      <button className="settings-save" onClick={() => onSave(draft)}>Save</button>
      <p className="settings-hint">
        Stored in this browser only. Generate one with <code>bordo platform token</code>.
      </p>
    </div>
  )
}

function Shell() {
  const { token, saveToken } = useAuth()
  const location = useLocation()
  const [activeChatId, setActiveChatId] = useState<string | null>(() => readStore(ACTIVE_CHAT_KEY))
  const [dockOpen, setDockOpen] = useState(() => readStore(DOCK_KEY) !== 'false')
  const [showActivity, setShowActivity] = useState(false)
  const [actionEvents, setActionEvents] = useState<ActionEvent[]>([])
  const [pending, setPending] = useState<{ chatId: string; text: string } | null>(null)
  const { chats, loaded, createChat, deleteChat, refetch } = useChats()

  const onHome = location.pathname === '/'
  const activeChat = chats.find((c) => c.id === activeChatId)

  const { messages, send, connected, busy } = useWebSocket(AGENT_WS_URL, token, activeChatId, (msg: WSIncoming) => {
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
        const i = prev.findIndex((e) => e.id === ev.id)
        if (i < 0) return [ev, ...prev]
        const next = [...prev]
        next[i] = ev
        return next
      })
    } else if (msg.type === 'done') {
      void refetch() // the server titles a chat from its first message
    }
  })

  const selectChat = useCallback((id: string | null) => {
    setActiveChatId(id)
    writeStore(ACTIVE_CHAT_KEY, id)
  }, [])

  // A remembered chat that the server no longer has (deleted elsewhere, or the server was
  // reset) would otherwise keep receiving messages that never show up in the list.
  useEffect(() => {
    if (loaded && activeChatId && !chats.some((c) => c.id === activeChatId) && pending?.chatId !== activeChatId) {
      selectChat(null)
    }
  }, [loaded, chats, activeChatId, pending, selectChat])

  // Deliver a message that created its own chat once that chat's socket is up.
  useEffect(() => {
    if (pending && connected && pending.chatId === activeChatId) {
      send(pending.text)
      setPending(null)
    }
  }, [pending, connected, activeChatId, send])

  const handleSend = useCallback(
    async (text: string) => {
      if (activeChatId) {
        send(text)
        return
      }
      const session = await createChat()
      if (!session) return
      setPending({ chatId: session.id, text })
      selectChat(session.id)
    },
    [activeChatId, send, createChat, selectChat],
  )

  // Pages use this to hand the agent a task ("change the code", ...); it also reveals the dock.
  const askAgent = useCallback(
    (text: string) => {
      setDockOpen(true)
      writeStore(DOCK_KEY, 'true')
      void handleSend(text)
    },
    [handleSend],
  )

  const newChat = async () => {
    const session = await createChat()
    if (session) selectChat(session.id)
  }

  const removeChat = async (id: string) => {
    await deleteChat(id)
    if (id === activeChatId) selectChat(null)
  }

  const toggleDock = () => {
    setDockOpen((o) => {
      writeStore(DOCK_KEY, String(!o))
      return !o
    })
  }

  const decide = async (id: string, verdict: 'approve' | 'reject') => {
    try {
      await fetch(`/api/approvals/${id}/${verdict}`, { method: 'POST' })
    } catch {
      // surfaced by the status staying unchanged
    }
    setActionEvents((prev) => prev.map((e) => (e.id === id ? { ...e, status: verdict === 'approve' ? 'approved' : 'failed' } : e)))
  }

  const showDock = !onHome && dockOpen

  return (
    <div className="app-layout">
      <Sidebar />
      <main className="app-main">
        <div className="app-body">
          <div className="app-content">
            <Routes>
              <Route
                path="/"
                element={
                  <ChatPage
                    chats={chats}
                    activeChatId={activeChatId}
                    onSelect={selectChat}
                    onNew={() => void newChat()}
                    onDelete={(id) => void removeChat(id)}
                    messages={messages}
                    onSend={(t) => void handleSend(t)}
                    connected={connected}
                    busy={busy}
                    title={activeChat?.title}
                  />
                }
              />
              <Route path="/apps" element={<AppsPage />} />
              <Route path="/apps/:name/*" element={<AppDetailPage onAsk={askAgent} />} />
              <Route path="/regions" element={<RegionsPage />} />
              <Route path="/observe" element={<ObservePage />} />
              <Route path="/settings" element={<div className="app-content"><SettingsPanel token={token} onSave={saveToken} /></div>} />
              {/* old routes */}
              <Route path="/services/*" element={<Navigate to="/apps" replace />} />
              <Route path="/templates" element={<Navigate to="/apps" replace />} />
              <Route path="/build" element={<Navigate to="/apps" replace />} />
              <Route path="/release" element={<Navigate to="/apps" replace />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </div>

          {!onHome && !dockOpen && (
            <button className="dock-fab" onClick={toggleDock}><Sparkles size={15} aria-hidden="true" /> Ask Bordo</button>
          )}

          {showDock && (
            <div className="dock">
              <div className="dock-head">
                <span className="dock-title"><Logo size={16} /> Bordo</span>
                <select
                  className="select dock-select"
                  value={activeChatId ?? ''}
                  onChange={(e) => selectChat(e.target.value || null)}
                  aria-label="Conversation"
                >
                  <option value="">New conversation</option>
                  {chats.map((c) => <option key={c.id} value={c.id}>{c.title || 'New chat'}</option>)}
                </select>
                <button className="icon-btn" onClick={() => selectChat(null)} title="New conversation" aria-label="New conversation">
                  <Plus size={15} aria-hidden="true" />
                </button>
                <button className="icon-btn" onClick={() => setShowActivity((v) => !v)} title="Agent activity" aria-label="Agent activity" aria-pressed={showActivity}>
                  <ListChecks size={15} aria-hidden="true" />
                  {actionEvents.some((e) => e.status === 'pending') && <span className="icon-btn-dot" aria-label="pending approval" />}
                </button>
                <button className="icon-btn" onClick={toggleDock} title="Hide chat" aria-label="Hide chat">
                  <X size={15} aria-hidden="true" />
                </button>
              </div>
              <ChatPane
                variant="dock"
                messages={messages}
                onSend={(t) => void handleSend(t)}
                connected={connected}
                busy={busy}
                chatId={activeChatId}
                title={activeChat?.title}
              />
            </div>
          )}

          {showActivity && (
            <ActivityPanel
              events={actionEvents}
              onApprove={(id) => void decide(id, 'approve')}
              onReject={(id) => void decide(id, 'reject')}
              onClose={() => setShowActivity(false)}
            />
          )}
        </div>
      </main>
    </div>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <SetupGate>
        <Shell />
      </SetupGate>
    </BrowserRouter>
  )
}
