import { useCallback, useEffect, useRef, useState } from 'react'

/**
 * Runs `fn` on mount and whenever `deps` change, optionally polling.
 * Keeps the previous data visible while a refresh is in flight.
 */
export function useFetch<T>(fn: () => Promise<T>, deps: unknown[], pollMs = 0) {
  const [data, setData] = useState<T | undefined>(undefined)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const fnRef = useRef(fn)
  fnRef.current = fn
  const seq = useRef(0)

  const run = useCallback(async () => {
    const id = ++seq.current
    try {
      const d = await fnRef.current()
      if (id === seq.current) {
        setData(d)
        setError(null)
      }
    } catch (e) {
      if (id === seq.current) setError(e instanceof Error ? e.message : 'Request failed')
    } finally {
      if (id === seq.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    setLoading(true)
    void run()
    if (!pollMs) return
    const t = setInterval(() => void run(), pollMs)
    return () => {
      clearInterval(t)
      seq.current++
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, pollMs, run])

  return { data, error, loading, reload: run }
}
