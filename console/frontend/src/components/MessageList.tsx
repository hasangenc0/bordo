import { useEffect, useRef } from 'react'
import type { Message } from '../types'
import { Markdown } from './Markdown'
import { ToolCard } from './ToolCard'

interface Props {
  messages: Message[]
  busy: boolean
}

export function MessageList({ messages, busy }: Props) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const stickRef = useRef(true)

  // Follow the conversation only while the user hasn't scrolled up to read.
  useEffect(() => {
    const el = scrollRef.current
    if (el && stickRef.current) el.scrollTop = el.scrollHeight
  }, [messages, busy])

  const lastIsTool = messages.length > 0 && !!messages[messages.length - 1].tool
  const lastTool = messages[messages.length - 1]?.tool
  const showThinking = busy && !(lastIsTool && lastTool?.status === 'running')

  return (
    <div
      className="message-list"
      ref={scrollRef}
      onScroll={(e) => {
        const el = e.currentTarget
        stickRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
      }}
    >
      {messages.map((msg) =>
        msg.tool ? (
          <div key={msg.id} className="message message--tool">
            <ToolCard tool={msg.tool} />
          </div>
        ) : (
          <div key={msg.id} className={`message message--${msg.role}`}>
            <div className={`message-bubble${msg.isError ? ' message-bubble--error' : ''}`}>
              {msg.role === 'assistant' ? <Markdown text={msg.content} /> : <p className="message-text">{msg.content}</p>}
            </div>
          </div>
        ),
      )}
      {showThinking && (
        <div className="message message--assistant">
          <div className="message-bubble thinking" aria-label="Bordo is thinking"><span /><span /><span /></div>
        </div>
      )}
    </div>
  )
}
