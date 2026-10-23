import { useRef, useState } from 'react'
import type { Message } from '../types'
import { MessageList } from './MessageList'
import { ArrowUp } from 'lucide-react'
import { Logo } from './Logo'

interface Props {
  messages: Message[]
  onSend: (content: string) => void
  connected: boolean
  busy: boolean
  chatId: string | null
  title?: string
  /** 'page' is the full-width home chat; 'dock' is the side panel on other pages. */
  variant?: 'page' | 'dock'
  suggestions?: string[]
}

export const DEFAULT_SUGGESTIONS = [
  'What apps do I have running?',
  'Create a java web service called hello-api',
  'Show me CPU and memory for my apps',
  'Add a /health endpoint to my app and open a PR',
]

export function ChatPane({ messages, onSend, connected, busy, chatId, title, variant = 'dock', suggestions = DEFAULT_SUGGESTIONS }: Props) {
  const [draft, setDraft] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  // A brand-new conversation has no socket until the first message creates it,
  // so the input must stay usable then.
  const canType = !chatId || connected

  const submit = (text = draft) => {
    const t = text.trim()
    if (!t || !canType || busy) return
    onSend(t)
    setDraft('')
    if (textareaRef.current) textareaRef.current.style.height = ''
    textareaRef.current?.focus()
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      submit()
    }
  }

  const empty = messages.length === 0

  return (
    <div className={`chat-pane chat-pane--${variant}`}>
      {chatId && (
        <div className="chat-header">
          {title && <span className="chat-header-title">{title}</span>}
          <span className={`status-dot ${connected ? 'status-dot--connected' : 'status-dot--disconnected'}`} />
          <span className="chat-status-text">{connected ? 'Connected' : 'Connecting…'}</span>
        </div>
      )}

      {empty ? (
        <div className="chat-welcome">
          <div className="chat-empty-icon"><Logo size={52} /></div>
          <h2 className="chat-empty-title">What do you want to build?</h2>
          <p className="chat-empty-hint">Create apps, ship them to your regions, check metrics, or change code. Just ask.</p>
          <div className="suggestions">
            {suggestions.map((q) => (
              <button key={q} type="button" className="suggestion" onClick={() => submit(q)} disabled={!canType}>
                {q}
              </button>
            ))}
          </div>
        </div>
      ) : (
        <MessageList messages={messages} busy={busy} />
      )}

      <div className="composer-wrap">
        <div className="composer">
          <textarea
            ref={textareaRef}
            className="composer-input"
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value)
              const el = e.currentTarget
              el.style.height = 'auto'
              el.style.height = `${Math.min(el.scrollHeight, 200)}px`
            }}
            onKeyDown={handleKeyDown}
            placeholder={canType ? 'Ask Bordo to build, deploy or change something…' : 'Connecting to agent…'}
            disabled={!canType}
            rows={1}
            aria-label="Message Bordo"
          />
          <div className="composer-bar">
            <span className="composer-hint">
              {busy ? 'Working…' : <><kbd>Enter</kbd> to send · <kbd>Shift</kbd> <kbd>Enter</kbd> for a new line</>}
            </span>
            <button
              type="button"
              className={`send-btn${busy ? ' send-btn--busy' : ''}`}
              onClick={() => submit()}
              disabled={!canType || busy || !draft.trim()}
              aria-label={busy ? 'Working' : 'Send message'}
            >
              {busy ? <span className="spinner spinner--inverse" aria-hidden="true" /> : <ArrowUp size={16} strokeWidth={2.25} aria-hidden="true" />}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
