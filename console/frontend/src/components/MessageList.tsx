import { useEffect, useRef } from 'react'
import type { Message } from '../types'
import { ToolResultCard } from './ToolResultCard'

interface Props {
  messages: Message[]
}

export function MessageList({ messages }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (messages.length === 0) {
    return (
      <div className="message-list message-list--empty">
        <p>Ask Bordo anything — scaffold a service, trigger a build, check a deploy, query metrics.</p>
      </div>
    )
  }

  return (
    <div className="message-list">
      {messages.map((msg) => (
        <div key={msg.id} className={`message message--${msg.role}`}>
          <div className="message-bubble">
            {msg.toolResult ? (
              <>
                {msg.content && <p className="message-text">{msg.content}</p>}
                <ToolResultCard toolName={msg.toolResult.toolName} data={msg.toolResult.data} />
              </>
            ) : (
              <p className="message-text">{msg.content}</p>
            )}
          </div>
        </div>
      ))}
      <div ref={bottomRef} />
    </div>
  )
}
