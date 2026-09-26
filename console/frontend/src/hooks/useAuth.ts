import { useState } from 'react'

export function useAuth() {
  const [token, setToken] = useState<string>(() => {
    try {
      return localStorage.getItem('bordo_token') ?? ''
    } catch {
      return ''
    }
  })

  const saveToken = (t: string) => {
    try {
      localStorage.setItem('bordo_token', t)
    } catch {
      // ignore storage errors (private mode, etc.)
    }
    setToken(t)
  }

  return { token, saveToken }
}
