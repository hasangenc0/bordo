import { useState } from 'react'
import { api } from '../api'
import { CopyButton } from './CopyButton'
import { Modal } from './Modal'

interface Props {
  onClose: () => void
  onAdded: () => void
}

const NAME_RE = /^[a-z][a-z0-9-]{1,30}[a-z0-9]$/

/** Registers a region and mints the one-time command the user runs on the target machine. */
export function AddRegionModal({ onClose, onAdded }: Props) {
  const [name, setName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<{ token: string; expires_at: string } | null>(null)

  const cpUrl = `${window.location.protocol}//${window.location.hostname}:7401`

  const submit = async () => {
    if (!NAME_RE.test(name)) return
    setSubmitting(true)
    setError(null)
    try {
      await api('/regions', { json: { name } })
      const tok = await api<{ token: string; expires_at: string }>(`/regions/${name}/join-tokens`, { json: {} })
      setResult(tok)
      onAdded()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not add region')
    } finally {
      setSubmitting(false)
    }
  }

  const command = result
    ? `curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install-node.sh | \\\n  BORDO_CP_URL=${cpUrl} \\\n  BORDO_JOIN_TOKEN=${result.token} sh`
    : ''

  return (
    <Modal
      title={result ? `Connect ${name}` : 'Add region'}
      onClose={onClose}
      footer={
        result ? (
          <button className="btn btn-primary" onClick={onClose}>Done</button>
        ) : (
          <>
            <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" onClick={() => void submit()} disabled={!NAME_RE.test(name) || submitting}>
              {submitting ? 'Adding…' : 'Add region'}
            </button>
          </>
        )
      }
    >
      {!result ? (
        <div className="form-group">
          <label className="form-label">Region name</label>
          <input className="input" autoFocus placeholder="us-east" value={name} onChange={(e) => setName(e.target.value.toLowerCase())} />
          <div className="muted" style={{ marginTop: 6 }}>
            A region is one machine (or cluster) that runs your apps. You'll get a command to run on it.
          </div>
          {error && <div className="notice notice--error">{error}</div>}
        </div>
      ) : (
        <>
          <p className="muted" style={{ marginBottom: 12 }}>
            Run this on the target machine. It installs k3s and connects back to Bordo over an outbound connection; no SSH keys are needed.
          </p>
          <div className="md-fence md-fence--wrap">
            <div className="md-fence-bar"><span>shell</span><CopyButton text={command} /></div>
            <pre>{command}</pre>
          </div>
          <div className="notice">
            The token works once and expires {result.expires_at ? new Date(result.expires_at).toLocaleString() : 'soon'}.
            The URL above is this console's host on port 7401; change it if the machine reaches Bordo at a different address.
            The region shows as healthy once the machine has joined.
          </div>
        </>
      )}
    </Modal>
  )
}
