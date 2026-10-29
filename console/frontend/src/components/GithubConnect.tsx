import { useEffect, useRef, useState } from 'react'
import { Check } from 'lucide-react'
import { api } from '../api'
import { CopyButton } from './CopyButton'

interface DeviceStart {
  user_code: string
  verification_uri: string
  expires_in: number
}
interface DeviceStatus {
  status: 'idle' | 'pending' | 'connected' | 'expired' | 'denied' | 'error'
  login?: string
  warning?: string
  error?: string
}

interface Props {
  /** False when the server has no OAuth App client id: only the token form is offered. */
  deviceReady: boolean
  connected: boolean
  onConnected: () => void
}

/** Connects GitHub with the device flow (a code to enter at github.com), or a personal access token. */
export function GithubConnect({ deviceReady, connected, onConnected }: Props) {
  const [start, setStart] = useState<DeviceStart | null>(null)
  const [done, setDone] = useState<{ login?: string; warning?: string } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [showToken, setShowToken] = useState(!deviceReady)
  const [pat, setPat] = useState('')
  const onConnectedRef = useRef(onConnected)
  onConnectedRef.current = onConnected

  useEffect(() => {
    if (!start || done) return
    const t = setInterval(async () => {
      try {
        const st = await api<DeviceStatus>('/github/device/status')
        if (st.status === 'connected') {
          setDone({ login: st.login, warning: st.warning })
          onConnectedRef.current()
        } else if (st.status !== 'pending') {
          setStart(null)
          setError(st.error ?? 'GitHub login did not complete')
        }
      } catch {
        // keep polling; a failed request is usually a restart
      }
    }, 2000)
    return () => clearInterval(t)
  }, [start, done])

  const begin = async () => {
    setBusy(true)
    setError(null)
    try {
      setStart(await api<DeviceStart>('/github/device/start', { json: {} }))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not start the GitHub login')
    } finally {
      setBusy(false)
    }
  }

  const savePat = async () => {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ login?: string; warning?: string }>('/github/user-token', { json: { token: pat.trim() } })
      setPat('')
      setDone({ login: r.login, warning: r.warning })
      onConnectedRef.current()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not save the token')
    } finally {
      setBusy(false)
    }
  }

  if (done || connected) {
    return (
      <div className="gh-connect">
        <p className="gh-ok">
          <Check size={16} aria-hidden="true" /> GitHub connected{done?.login ? ` as ${done.login}` : ''}. Builds will run in GitHub Actions.
        </p>
        {done?.warning && <p className="form-error">{done.warning}</p>}
      </div>
    )
  }

  return (
    <div className="gh-connect">
      {start ? (
        <>
          <p>
            Open <a href={start.verification_uri} target="_blank" rel="noreferrer">{start.verification_uri}</a> and enter this code:
          </p>
          <div className="code-box">
            <code>{start.user_code}</code>
            <CopyButton text={start.user_code} />
          </div>
          <p className="muted">Waiting for you to authorize. This page updates by itself.</p>
        </>
      ) : (
        deviceReady && (
          <button className="btn btn-primary" onClick={() => void begin()} disabled={busy}>
            {busy ? 'Starting…' : 'Connect GitHub'}
          </button>
        )
      )}

      {!deviceReady && (
        <p className="muted">
          One-click login needs a GitHub OAuth App with Device Flow enabled. Set its client id as <code>BORDO_GITHUB_CLIENT_ID</code> in
          <code> ~/.bordo/deploy/.env</code> and run <code>bordo platform up</code>. Until then, paste a token.
        </p>
      )}

      {!start && deviceReady && !showToken && (
        <button className="link-btn" onClick={() => setShowToken(true)}>Use a personal access token instead</button>
      )}
      {showToken && !start && (
        <div className="form-group">
          <label className="form-label" htmlFor="gh-pat">Personal access token (classic: repo, workflow, write:packages)</label>
          <input
            id="gh-pat"
            type="password"
            className="settings-input"
            value={pat}
            onChange={(e) => setPat(e.target.value)}
            autoComplete="off"
            placeholder="ghp_…"
          />
          <button className="btn btn-secondary" onClick={() => void savePat()} disabled={busy || !pat.trim()}>
            Save token
          </button>
        </div>
      )}
      {error && <p className="form-error" role="alert">{error}</p>}
    </div>
  )
}
