import { useEffect, useState } from 'react'
import { api } from '../api'
import { CopyButton } from './CopyButton'
import { Modal } from './Modal'

interface Props {
  /** Existing region to mint a fresh join command for; omit to register a new one. */
  region?: string
  onClose: () => void
  onAdded: () => void
}

const NAME_RE = /^[a-z][a-z0-9-]{1,30}[a-z0-9]$/
const CP_URL_KEY = 'bordo_cp_url'

function storedUrl(): string {
  try {
    return localStorage.getItem(CP_URL_KEY) ?? ''
  } catch {
    return ''
  }
}

// The address the *new machine* uses to reach the control plane. When the console is
// opened through a tunnel (localhost) the browser's own host is useless to it.
function defaultCpUrl(): string {
  const saved = storedUrl()
  if (saved) return saved
  const h = window.location.hostname
  return h && h !== 'localhost' && h !== '127.0.0.1' ? `http://${h}:7401` : ''
}

/** Registers a region (or reuses an existing one) and mints the one-time command to run on the target machine. */
export function AddRegionModal({ region, onClose, onAdded }: Props) {
  const [name, setName] = useState(region ?? '')
  const [cpUrl, setCpUrl] = useState(defaultCpUrl)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<{ token: string; expires_at: string } | null>(null)

  const mint = async (regionName: string, register: boolean) => {
    setSubmitting(true)
    setError(null)
    try {
      if (register) await api('/regions', { json: { name: regionName } })
      setResult(await api<{ token: string; expires_at: string }>(`/regions/${regionName}/join-tokens`, { json: {} }))
      onAdded()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not get a join command')
    } finally {
      setSubmitting(false)
    }
  }

  // Existing region: mint straight away, there is nothing to fill in first.
  useEffect(() => {
    if (region) void mint(region, false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [region])

  const updateUrl = (v: string) => {
    setCpUrl(v)
    try {
      localStorage.setItem(CP_URL_KEY, v)
    } catch {
      // not persisted in private mode; the field still works
    }
  }

  const base = cpUrl.trim().replace(/\/+$/, '')
  const command = result
    ? `curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install-node.sh | \\\n  BORDO_CP_URL=${base || 'http://<SERVER-IP>:7401'} \\\n  BORDO_JOIN_TOKEN=${result.token} sh`
    : ''
  const ready = !!result

  return (
    <Modal
      title={ready ? `Connect ${name}` : region ? `Join command for ${region}` : 'Add region'}
      onClose={onClose}
      footer={
        ready ? (
          <button className="btn btn-primary" onClick={onClose}>Done</button>
        ) : (
          <>
            <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
            {!region && (
              <button className="btn btn-primary" onClick={() => void mint(name, true)} disabled={!NAME_RE.test(name) || submitting}>
                {submitting ? 'Adding…' : 'Add region'}
              </button>
            )}
          </>
        )
      }
    >
      {!ready ? (
        <div className="form-group">
          {region ? (
            <div className="muted">{submitting ? 'Creating a fresh one-time token…' : 'Could not create a token.'}</div>
          ) : (
            <>
              <label className="form-label">Region name</label>
              <input className="input" autoFocus placeholder="eu-central" value={name} onChange={(e) => setName(e.target.value.toLowerCase())} />
              <div className="muted" style={{ marginTop: 6 }}>
                A region is one machine (or cluster) that runs your apps. You'll get a command to run on it.
              </div>
            </>
          )}
          {error && <div className="notice notice--error">{error}</div>}
        </div>
      ) : (
        <>
          <div className="form-group">
            <label className="form-label">Bordo server address, as the new machine sees it</label>
            <input className="input mono" placeholder="http://<SERVER-IP>:7401" value={cpUrl} onChange={(e) => updateUrl(e.target.value)} />
            {!base && <div className="notice notice--error" style={{ marginTop: 8 }}>Enter your server's public address (port 7401) so the command works.</div>}
          </div>
          <p className="muted" style={{ marginBottom: 12 }}>
            Run this on the target machine. It installs k3s and connects back to Bordo over an outbound connection; no SSH keys are needed.
          </p>
          <div className="md-fence md-fence--wrap">
            <div className="md-fence-bar"><span>shell</span><CopyButton text={command} /></div>
            <pre>{command}</pre>
          </div>
          <div className="notice">
            The token works once and expires {result.expires_at ? new Date(result.expires_at).toLocaleString() : 'soon'}.
            The machine must be able to reach the address above (check the server's firewall for port 7401).
            The region shows as healthy once the machine has joined.
          </div>
        </>
      )}
    </Modal>
  )
}
