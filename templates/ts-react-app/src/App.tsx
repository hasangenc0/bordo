import { useState, useEffect } from 'react'

const API_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

export default function App() {
  const [message, setMessage] = useState<string | null>(null)

  useEffect(() => {
    fetch(`${API_URL}/api/v1/hello`)
      .then((r) => r.json())
      .then((d) => setMessage(d.message ?? JSON.stringify(d)))
      .catch(() => setMessage('backend unreachable'))
  }, [])

  return (
    <main style={{ fontFamily: 'sans-serif', padding: '2rem' }}>
      <h1>{{.ProjectName}}</h1>
      <p>{message ?? 'loading…'}</p>
    </main>
  )
}
