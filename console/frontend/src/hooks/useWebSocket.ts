import { useCallback, useEffect, useRef, useState } from 'react'
import type { Message, WSIncoming, WSOutgoing } from '../types'

const RECONNECT_DELAY_MS = 3000

export function useWebSocket(agentURL: string, token: string) {
  const [messages, setMessages] = useState<Message[]>([])
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const intentionalClose = useRef(false)

  const connect = useCallback(() => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) return

    const ws = new WebSocket(agentURL)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
    }

    ws.onmessage = (evt) => {
      try {
        const msg: WSIncoming = JSON.parse(evt.data as string)
        if (msg.type === 'message' || msg.type === 'error') {
          setMessages((prev) => [
            ...prev,
            {
              id: `${Date.now()}-${Math.random()}`,
              role: 'assistant',
              content: msg.content,
              timestamp: Date.now(),
            },
          ])
        } else if (msg.type === 'tool_result' && msg.tool_name) {
          setMessages((prev) => [
            ...prev,
            {
              id: `${Date.now()}-${Math.random()}`,
              role: 'assistant',
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
      if (!intentionalClose.current) {
        reconnectTimer.current = setTimeout(connect, RECONNECT_DELAY_MS)
      }
    }

    ws.onerror = () => {
      ws.close()
    }
  }, [agentURL])

  useEffect(() => {
    intentionalClose.current = false
    connect()
    return () => {
      intentionalClose.current = true
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
    }
  }, [connect])

  const send = useCallback(
    (content: string) => {
      if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return
      const payload: WSOutgoing = { type: 'message', content, token }
      wsRef.current.send(JSON.stringify(payload))
      setMessages((prev) => [
        ...prev,
        {
          id: `${Date.now()}-${Math.random()}`,
          role: 'user',
          content,
          timestamp: Date.now(),
        },
      ])
    },
    [token],
  )

  return { messages, send, connected }
}
