import { useRef, useState } from 'react'
import type { Message } from '../types'
import { MessageList } from './MessageList'

interface Props {
  messages: Message[]
  onSend: (content: string) => void
  connected: boolean
  chatId: string | null
  title?: string
}

export function ChatPane({ messages, onSend, connected, chatId, title }: Props) {
  const [draft, setDraft] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  if (!chatId) {
    return (
      <div className="chat-pane chat-pane--empty">
        <div className="chat-empty-state">
          <div className="chat-empty-icon">✦</div>
          <h2 className="chat-empty-title">Bordo Agent</h2>
          <p className="chat-empty-hint">Create a new chat to get started</p>
        </div>
      </div>
    )
  }

  const submit = () => {
    const text = draft.trim()
    if (!text || !connected) return
    onSend(text)
    setDraft('')
    textareaRef.current?.focus()
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  return (
    <div className="chat-pane">
      <div className="chat-header">
        {title && <span className="chat-header-title">{title}</span>}
        <span className={`status-dot ${connected ? 'status-dot--connected' : 'status-dot--disconnected'}`} />
        <span className="chat-status-text">{connected ? 'Connected' : 'Connecting…'}</span>
      </div>
      {messages.length === 0 && connected ? (
        <div className="message-list message-list--empty">
          <p>Ask Bordo anything about your platform…</p>
        </div>
      ) : (
        <MessageList messages={messages} />
      )}
      <div className="chat-input-row">
        <textarea
          ref={textareaRef}
          className="chat-input"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={connected ? 'Ask Bordo… (Enter to send, Shift+Enter for newline)' : 'Connecting to agent…'}
          disabled={!connected}
          rows={3}
        />
        <button className="chat-send" onClick={submit} disabled={!connected || !draft.trim()}>
          Send
        </button>
      </div>
    </div>
  )
}
