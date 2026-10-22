import { useState } from 'react'

export function CopyButton({ text, label = 'Copy' }: { text: string; label?: string }) {
  const [done, setDone] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      // clipboard is unavailable on plain-http origins; fall back to a hidden textarea
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      try {
        document.execCommand('copy')
      } catch {
        return
      } finally {
        document.body.removeChild(ta)
      }
    }
    setDone(true)
    setTimeout(() => setDone(false), 1500)
  }
  return (
    <button type="button" className="copy-btn" onClick={() => void copy()}>
      {done ? 'Copied' : label}
    </button>
  )
}
