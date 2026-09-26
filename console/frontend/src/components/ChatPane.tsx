import { useRef, useState } from 'react'
import type { Message } from '../types'
import { MessageList } from './MessageList'

interface Props {
  messages: Message[]
  onSend: (content: string) => void
  connected: boolean
}

export function ChatPane({ messages, onSend, connected }: Props) {
  const [draft, setDraft] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

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
      <div className="chat-status">
        <span className={`status-dot ${connected ? 'status-dot--connected' : 'status-dot--disconnected'}`} />
        {connected ? 'Connected' : 'Connecting…'}
      </div>
      <MessageList messages={messages} />
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
