import { useCallback, useEffect, useRef, useState } from 'react'
import type { Message, WSIncoming, WSOutgoing } from '../types'

const RECONNECT_DELAY_MS = 3000

let seq = 0
const nextId = (prefix: string) => `${prefix}-${Date.now()}-${seq++}`

export function useWebSocket(
  baseURL: string,
  token: string,
  chatId: string | null,
  onEvent?: (msg: WSIncoming) => void,
) {
  const [messages, setMessages] = useState<Message[]>([])
  const [connected, setConnected] = useState(false)
  const [busy, setBusy] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent
  const tokenRef = useRef(token)
  tokenRef.current = token
  // Messages sent before the socket opened (e.g. the first message of a brand-new chat).
  const queueRef = useRef<string[]>([])
  // Incremented whenever we switch chats; captured per-WS so stale sockets don't reconnect.
  const epochRef = useRef(0)

  const agentURL = chatId ? `${baseURL}?chat_id=${chatId}` : null

  const transmit = useCallback((ws: WebSocket, content: string) => {
    const payload: WSOutgoing = { type: 'message', content, token: tokenRef.current }
    ws.send(JSON.stringify(payload))
    setBusy(true)
    setMessages((prev) => [
      ...prev,
      { id: nextId('local'), role: 'user', content, timestamp: Date.now() },
    ])
  }, [])

  const connect = useCallback(
    (epoch: number) => {
      if (!agentURL) return
      if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) return

      const ws = new WebSocket(agentURL)
      // Some proxies hand text frames over as binary; ArrayBuffer lets us decode them
      // synchronously (an async Blob read could reorder frames).
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        setConnected(true)
        const queued = queueRef.current.splice(0)
        for (const q of queued) transmit(ws, q)
      }

      ws.onmessage = (evt) => {
        let msg: WSIncoming
        try {
          const raw = typeof evt.data === 'string' ? evt.data : new TextDecoder().decode(evt.data as ArrayBuffer)
          msg = JSON.parse(raw) as WSIncoming
        } catch {
          return // ignore malformed frames
        }
        onEventRef.current?.(msg)

        switch (msg.type) {
          case 'running':
            // The agent is still on a turn started earlier (this client reconnected mid-task).
            setBusy(true)
            break
          case 'history':
            setMessages((prev) => [
              ...(msg.messages ?? []).map((m, i) => ({
                id: `history-${i}-${m.role}`,
                role: m.role,
                content: m.content,
                timestamp: Date.now() - ((msg.messages?.length ?? 0) - i) * 1000,
              })),
              // keep anything the user sent while the history was in flight
              ...prev.filter((m) => m.id.startsWith('local-')),
            ])
            break
          case 'message':
            if (!msg.content) break
            setMessages((prev) => [
              ...prev,
              { id: nextId('a'), role: 'assistant', content: msg.content ?? '', timestamp: Date.now() },
            ])
            break
          case 'error':
            setBusy(false)
            setMessages((prev) => [
              ...prev,
              { id: nextId('e'), role: 'assistant', content: msg.content ?? 'Something went wrong', isError: true, timestamp: Date.now() },
            ])
            break
          case 'tool_call':
            setMessages((prev) => [
              ...prev,
              {
                id: nextId('t'),
                role: 'assistant',
                content: '',
                tool: { name: msg.tool_name ?? 'tool', input: msg.input, status: 'running' },
                timestamp: Date.now(),
              },
            ])
            break
          case 'tool_result':
            setMessages((prev) => {
              // Resolve the oldest still-running call of this tool (calls finish in order).
              const idx = prev.findIndex((m) => m.tool?.status === 'running' && m.tool.name === msg.tool_name)
              if (idx < 0) return prev
              const next = [...prev]
              const t = next[idx].tool!
              next[idx] = {
                ...next[idx],
                tool: { ...t, status: msg.error ? 'error' : 'done', data: msg.data, error: msg.error },
              }
              return next
            })
            break
          case 'done':
            setBusy(false)
            // Any call still "running" at the end of a turn never reported back.
            setMessages((prev) =>
              prev.some((m) => m.tool?.status === 'running')
                ? prev.map((m) => (m.tool?.status === 'running' ? { ...m, tool: { ...m.tool, status: 'done' as const } } : m))
                : prev,
            )
            break
        }
      }

      ws.onclose = () => {
        setConnected(false)
        setBusy(false)
        // Only reconnect if we're still on the same chat session (epoch unchanged).
        if (epochRef.current === epoch) {
          reconnectTimer.current = setTimeout(() => connect(epoch), RECONNECT_DELAY_MS)
        }
      }

      ws.onerror = () => ws.close()
    },
    [agentURL, transmit],
  )

  useEffect(() => {
    // New epoch invalidates any pending reconnects from the previous chat.
    epochRef.current += 1
    const epoch = epochRef.current

    setMessages([])
    setConnected(false)
    setBusy(false)
    queueRef.current = [] // never deliver a queued message into a different chat
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
      const ws = wsRef.current
      if (ws && ws.readyState === WebSocket.OPEN) transmit(ws, content)
      else queueRef.current.push(content)
    },
    [transmit],
  )

  // Ask the agent to abandon the turn in progress. The agent answers with a final 'done' frame.
  const stop = useCallback(() => {
    const ws = wsRef.current
    if (ws && ws.readyState === WebSocket.OPEN) {
      const payload: WSOutgoing = { type: 'stop', token: tokenRef.current }
      ws.send(JSON.stringify(payload))
    }
  }, [])

  return { messages, send, stop, connected, busy }
}
