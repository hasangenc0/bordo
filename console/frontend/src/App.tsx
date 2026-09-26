import { useState } from 'react'
import type { SidebarSection } from './types'
import { Sidebar } from './components/Sidebar'
import { ChatPane } from './components/ChatPane'
import { useAuth } from './hooks/useAuth'
import { useWebSocket } from './hooks/useWebSocket'
import './styles.css'

const AGENT_WS_URL = import.meta.env.VITE_AGENT_WS_URL ?? 'ws://localhost:7402/ws/chat'

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
  const { messages, send, connected } = useWebSocket(AGENT_WS_URL, token)
  const [section, setSection] = useState<SidebarSection>('projects')

  const handleSectionSelect = (s: SidebarSection) => {
    setSection(s)
    const prompt = SECTION_PROMPTS[s]
    if (prompt && connected) {
      send(prompt)
    }
  }

  return (
    <div className="app-layout">
      <Sidebar active={section} onSelect={handleSectionSelect} />
      <main className="app-main">
        {section === 'settings' ? (
          <SettingsPanel token={token} onSave={saveToken} />
        ) : (
          <ChatPane messages={messages} onSend={send} connected={connected} />
        )}
      </main>
    </div>
  )
}
