import { useState } from 'react'
import type { SidebarSection } from './types'
import { Sidebar } from './components/Sidebar'
import { ChatPane } from './components/ChatPane'
import { useAuth } from './hooks/useAuth'
import { useWebSocket } from './hooks/useWebSocket'
import { useChats } from './hooks/useChats'
import './styles.css'

const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
const AGENT_WS_URL =
  (import.meta.env.VITE_AGENT_WS_URL as string | undefined) ??
  `${wsProto}//${window.location.host}/ws/chat`

const SECTION_PROMPTS: Record<SidebarSection, string | null> = {
  projects: '[Projects context: list my projects]',
  build: '[Build context: show my recent builds]',
  release: '[Release context: show my recent deploys]',
  observe: '[Observe context: show recent metrics and alerts]',
  metrics: '[Metrics context: show key metrics for my projects]',
  settings: null,
}

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

export default function App() {
  const { token, saveToken } = useAuth()
  const [section, setSection] = useState<SidebarSection>('projects')
  const [activeChatId, setActiveChatId] = useState<string | null>(null)
  const { chats, loading: chatsLoading, createChat, deleteChat, updateChatTitle } = useChats()

  const activeChat = chats.find((c) => c.id === activeChatId)
  const { messages, send, connected } = useWebSocket(AGENT_WS_URL, token, activeChatId)

  const handleNewChat = async () => {
    const session = await createChat()
    if (session) {
      setActiveChatId(session.id)
      setSection('projects')
    }
  }

  const handleChatSelect = (id: string) => {
    setActiveChatId(id)
  }

  const handleDeleteChat = async (id: string) => {
    await deleteChat(id)
    if (activeChatId === id) setActiveChatId(null)
  }

  const handleSectionSelect = (s: SidebarSection) => {
    setSection(s)
    const prompt = SECTION_PROMPTS[s]
    if (prompt && connected && activeChatId) {
      send(prompt)
      // Auto-title the chat from the first section context if it's still untitled
      if (activeChat && activeChat.title === 'New Chat') {
        updateChatTitle(activeChatId, s.charAt(0).toUpperCase() + s.slice(1) + ' overview')
      }
    }
  }

  return (
    <div className="app-layout">
      <Sidebar
        active={section}
        onSelect={handleSectionSelect}
        chats={chats}
        activeChatId={activeChatId}
        onChatSelect={handleChatSelect}
        onNewChat={handleNewChat}
        onDeleteChat={handleDeleteChat}
        chatsLoading={chatsLoading}
      />
      <main className="app-main">
        {section === 'settings' ? (
          <SettingsPanel token={token} onSave={saveToken} />
        ) : (
          <ChatPane
            messages={messages}
            onSend={send}
            connected={connected}
            chatId={activeChatId}
            title={activeChat?.title}
          />
        )}
      </main>
    </div>
  )
}
