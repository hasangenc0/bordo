import { useState, type ReactNode } from 'react'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import { GithubConnect } from './GithubConnect'
import { Logo } from './Logo'
import { Modal } from './Modal'

export interface SetupStatus {
  configured: boolean
  github_connected: boolean
  github_device_ready: boolean
}

const DISMISS_KEY = 'bordo_github_prompt_dismissed'

function dismissed(): boolean {
  try {
    return sessionStorage.getItem(DISMISS_KEY) === '1'
  } catch {
    return false
  }
}

/**
 * First-run flow inside the console: asks for the DeepSeek key until the server is
 * configured, then offers to connect GitHub. If the status call fails (an older server),
 * the console renders as before.
 */
export function SetupGate({ children }: { children: ReactNode }) {
  const { data, loading, reload } = useFetch(() => api<SetupStatus>('/setup/status'), [])
  const [hidden, setHidden] = useState(dismissed)
  const [modal, setModal] = useState(false)

  if (loading && !data) return null
  if (!data) return <>{children}</>
  if (!data.configured) return <FirstRun status={data} onDone={() => void reload()} />

  const showBanner = !data.github_connected && !hidden
  return (
    <>
      {showBanner && (
        <div className="setup-banner" role="status">
          <span>Connect GitHub so Bordo can build and deploy your code.</span>
          <button className="btn btn-primary btn-sm" onClick={() => setModal(true)}>Connect</button>
          <button
            className="btn btn-secondary btn-sm"
            onClick={() => {
              setHidden(true)
              try {
                sessionStorage.setItem(DISMISS_KEY, '1')
              } catch {
                // dismissal just won't persist
              }
            }}
          >
            Later
          </button>
        </div>
      )}
      {children}
      {modal && (
        <Modal title="Connect GitHub" onClose={() => setModal(false)}>
          <GithubConnect deviceReady={data.github_device_ready} connected={data.github_connected} onConnected={() => void reload()} />
        </Modal>
      )}
    </>
  )
}

function FirstRun({ status, onDone }: { status: SetupStatus; onDone: () => void }) {
  const [key, setKey] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  const submit = async () => {
    setBusy(true)
    setError(null)
    try {
      await api('/setup', { json: { deepseek_api_key: key.trim() } })
      setSaved(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Setup failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="setup-screen">
      <div className="setup-card">
        <div className="setup-brand"><Logo size={32} /><span>Bordo</span></div>
        {!saved ? (
          <>
            <h1>Finish setting up</h1>
            <p className="muted">
              The agent uses DeepSeek to plan and write code. Get a key at{' '}
              <a href="https://platform.deepseek.com/api_keys" target="_blank" rel="noreferrer">platform.deepseek.com</a>.
            </p>
            <form
              onSubmit={(e) => {
                e.preventDefault()
                void submit()
              }}
            >
              <label className="form-label" htmlFor="ds-key">DeepSeek API key</label>
              <input
                id="ds-key"
                type="password"
                className="settings-input"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                autoComplete="off"
                autoFocus
                placeholder="sk-…"
              />
              {error && <p className="form-error" role="alert">{error}</p>}
              <button className="btn btn-primary" type="submit" disabled={busy || !key.trim()}>
                {busy ? 'Saving…' : 'Continue'}
              </button>
            </form>
            {window.location.protocol === 'http:' && window.location.hostname !== 'localhost' && (
              <p className="muted">This page is on plain http, so the key crosses the network unencrypted. Prefer an SSH tunnel or HTTPS.</p>
            )}
          </>
        ) : (
          <>
            <h1>Connect GitHub</h1>
            <p className="muted">Bordo builds your code in GitHub Actions and publishes images to GitHub's registry.</p>
            <GithubConnect deviceReady={status.github_device_ready} connected={false} onConnected={() => undefined} />
            <button className="btn btn-secondary" onClick={onDone}>{'Open the console'}</button>
          </>
        )}
      </div>
    </div>
  )
}
