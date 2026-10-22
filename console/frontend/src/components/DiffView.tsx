import { useMemo, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

interface FileDiff {
  path: string
  lines: string[]
  added: number
  removed: number
}

const MAX_LINES_PER_FILE = 400

function parse(diff: string): FileDiff[] {
  const files: FileDiff[] = []
  let cur: FileDiff | null = null
  for (const line of diff.split('\n')) {
    if (line.startsWith('diff --git ')) {
      const m = /^diff --git a\/(.+?) b\/(.+)$/.exec(line)
      cur = { path: m ? m[2] : line.slice(11), lines: [], added: 0, removed: 0 }
      files.push(cur)
      continue
    }
    if (!cur) continue
    if (line.startsWith('index ') || line.startsWith('--- ') || line.startsWith('+++ ')) continue
    if (line.startsWith('+')) cur.added++
    else if (line.startsWith('-')) cur.removed++
    cur.lines.push(line)
  }
  return files
}

function FileBlock({ file, startOpen }: { file: FileDiff; startOpen: boolean }) {
  const [open, setOpen] = useState(startOpen)
  const shown = file.lines.slice(0, MAX_LINES_PER_FILE)
  return (
    <div className="diff-file">
      <button type="button" className="diff-file-head" onClick={() => setOpen((o) => !o)}>
        <span className="diff-caret">{open ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}</span>
        <span className="diff-path">{file.path}</span>
        <span className="diff-add">+{file.added}</span>
        <span className="diff-del">−{file.removed}</span>
      </button>
      {open && (
        <pre className="diff-body">
          {shown.map((l, i) => {
            const cls = l.startsWith('+') ? 'diff-line diff-line--add' : l.startsWith('-') ? 'diff-line diff-line--del' : l.startsWith('@@') ? 'diff-line diff-line--hunk' : 'diff-line'
            return <div key={i} className={cls}>{l || ' '}</div>
          })}
          {file.lines.length > shown.length && (
            <div className="diff-line diff-line--hunk">… {file.lines.length - shown.length} more lines</div>
          )}
        </pre>
      )}
    </div>
  )
}

export function DiffView({ diff, truncated }: { diff: string; truncated?: boolean }) {
  const files = useMemo(() => parse(diff), [diff])
  if (files.length === 0) return <div className="diff-empty">No changes.</div>
  return (
    <div className="diff">
      {files.map((f) => (
        <FileBlock key={f.path} file={f} startOpen={files.length <= 3} />
      ))}
      {truncated && <div className="diff-empty">Diff truncated; run it locally for the full change.</div>}
    </div>
  )
}
