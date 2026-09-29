import { useCallback, useEffect, useRef, useState } from 'react'
import type { Message, WSIncoming, WSOutgoing } from '../types'

const RECONNECT_DELAY_MS = 3000

export function useWebSocket(
  baseURL: string,
  token: string,
  chatId: string | null,
  onEvent?: (msg: WSIncoming) => void,
) {
  const [messages, setMessages] = useState<Message[]>([])
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent
  // Incremented whenever we switch chats; captured per-WS so stale sockets don't reconnect.
  const epochRef = useRef(0)

  const agentURL = chatId ? `${baseURL}?chat_id=${chatId}` : null

  const connect = useCallback(
    (epoch: number) => {
      if (!agentURL) return
      if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) return

      const ws = new WebSocket(agentURL)
      wsRef.current = ws

      ws.onopen = () => setConnected(true)

      ws.onmessage = (evt) => {
        try {
          const msg: WSIncoming = JSON.parse(evt.data as string)

          // Forward all events to the optional callback
          if (onEventRef.current) onEventRef.current(msg)

          if (msg.type === 'history' && msg.messages) {
            setMessages(
              msg.messages.map((m, i) => ({
                id: `history-${i}-${m.role}`,
                role: m.role,
                content: m.content,
                timestamp: Date.now() - (msg.messages!.length - i) * 1000,
              })),
            )
          } else if (msg.type === 'message' || msg.type === 'error') {
            setMessages((prev) => [
              ...prev,
              {
                id: `${Date.now()}-${Math.random()}`,
                role: 'assistant' as const,
                content: msg.content,
                timestamp: Date.now(),
              },
            ])
          } else if (msg.type === 'tool_result' && msg.tool_name) {
            setMessages((prev) => [
              ...prev,
              {
                id: `${Date.now()}-${Math.random()}`,
                role: 'assistant' as const,
                content: msg.content,
                toolResult: { toolName: msg.tool_name!, data: msg.data },
                timestamp: Date.now(),
              },
            ])
          }
        } catch {
          // ignore malformed frames
        }
      }

      ws.onclose = () => {
        setConnected(false)
        // Only reconnect if we're still on the same chat session (epoch unchanged).
        if (epochRef.current === epoch) {
          reconnectTimer.current = setTimeout(() => connect(epoch), RECONNECT_DELAY_MS)
        }
      }

      ws.onerror = () => ws.close()
    },
    [agentURL],
  )

  useEffect(() => {
    // New epoch invalidates any pending reconnects from the previous chat.
    epochRef.current += 1
    const epoch = epochRef.current

    setMessages([])
    setConnected(false)
    if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
    wsRef.current?.close()
    wsRef.current = null

    if (!agentURL) return

    connect(epoch)

    return () => {
      epochRef.current += 1 // invalidate reconnects on unmount too
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
      wsRef.current = null
    }
  }, [agentURL, connect])

  const send = useCallback(
    (content: string) => {
      if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return
      const payload: WSOutgoing = { type: 'message', content, token }
      wsRef.current.send(JSON.stringify(payload))
      setMessages((prev) => [
        ...prev,
        {
          id: `${Date.now()}-${Math.random()}`,
          role: 'user' as const,
          content,
          timestamp: Date.now(),
        },
      ])
    },
    [token],
  )

  return { messages, send, connected }
}
