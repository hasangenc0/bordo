import { useCallback, useEffect, useState } from 'react'
import type { ChatSession } from '../types'

export function useChats() {
  const [chats, setChats] = useState<ChatSession[]>([])
  const [loading, setLoading] = useState(false)

  const fetchChats = useCallback(async () => {
    setLoading(true)
    try {
      const r = await fetch('/api/chats')
      if (r.ok) {
        const data = (await r.json()) as { chats: ChatSession[] }
        setChats(data.chats ?? [])
      }
    } catch {
      // agent may not be reachable yet
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchChats()
  }, [fetchChats])

  const createChat = useCallback(async (): Promise<ChatSession | null> => {
    try {
      const r = await fetch('/api/chats', { method: 'POST' })
      if (!r.ok) return null
      const session = (await r.json()) as ChatSession
      setChats((prev) => [session, ...prev])
      return session
    } catch {
      return null
    }
  }, [])

  const deleteChat = useCallback(async (id: string) => {
    try {
      await fetch(`/api/chats/${id}`, { method: 'DELETE' })
      setChats((prev) => prev.filter((c) => c.id !== id))
    } catch {
      // ignore
    }
  }, [])

  const renameChat = useCallback(async (id: string, title: string) => {
    try {
      await fetch(`/api/chats/${id}/title`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title }),
      })
      setChats((prev) => prev.map((c) => (c.id === id ? { ...c, title } : c)))
    } catch {
      // ignore
    }
  }, [])

  const updateChatTitle = useCallback((id: string, title: string) => {
    setChats((prev) => prev.map((c) => (c.id === id ? { ...c, title } : c)))
  }, [])

  return { chats, loading, createChat, deleteChat, renameChat, refetch: fetchChats, updateChatTitle }
}
