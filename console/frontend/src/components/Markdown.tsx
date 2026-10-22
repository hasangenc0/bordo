import { Fragment, type ReactNode } from 'react'
import { CopyButton } from './CopyButton'

// Small, dependency-free renderer for the subset of Markdown models actually emit:
// fenced code, headings, lists, paragraphs, **bold**, `code` and http(s) links.
// Everything is rendered through React nodes, never as raw HTML.

const INLINE = /(`[^`\n]+`|\*\*[^*\n]+\*\*|\[[^\]\n]+\]\(https?:\/\/[^)\s]+\))/g

function inline(text: string): ReactNode[] {
  return text.split(INLINE).map((part, i) => {
    if (part.startsWith('`') && part.endsWith('`') && part.length > 2) {
      return <code key={i} className="md-code">{part.slice(1, -1)}</code>
    }
    if (part.startsWith('**') && part.endsWith('**') && part.length > 4) {
      return <strong key={i}>{part.slice(2, -2)}</strong>
    }
    const link = /^\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)$/.exec(part)
    if (link) {
      return (
        <a key={i} href={link[2]} target="_blank" rel="noreferrer noopener">
          {link[1]}
        </a>
      )
    }
    return <Fragment key={i}>{part}</Fragment>
  })
}

function textBlocks(src: string, keyBase: string): ReactNode[] {
  const out: ReactNode[] = []
  const lines = src.split('\n')
  let para: string[] = []
  let list: { ordered: boolean; items: string[] } | null = null

  const flushPara = () => {
    if (para.length) {
      out.push(<p key={`${keyBase}-p${out.length}`}>{inline(para.join(' '))}</p>)
      para = []
    }
  }
  const flushList = () => {
    if (list) {
      const Tag = list.ordered ? 'ol' : 'ul'
      out.push(
        <Tag key={`${keyBase}-l${out.length}`}>
          {list.items.map((it, i) => <li key={i}>{inline(it)}</li>)}
        </Tag>,
      )
      list = null
    }
  }

  for (const line of lines) {
    const bullet = /^\s*[-*]\s+(.*)$/.exec(line)
    const numbered = /^\s*\d+[.)]\s+(.*)$/.exec(line)
    const heading = /^#{1,4}\s+(.*)$/.exec(line)
    if (bullet || numbered) {
      flushPara()
      const ordered = !!numbered
      if (list && list.ordered !== ordered) flushList()
      if (!list) list = { ordered, items: [] }
      list.items.push((bullet ?? numbered)![1])
    } else if (heading) {
      flushPara()
      flushList()
      out.push(<p key={`${keyBase}-h${out.length}`} className="md-heading">{inline(heading[1])}</p>)
    } else if (line.trim() === '') {
      flushPara()
      flushList()
    } else {
      flushList()
      para.push(line.trim())
    }
  }
  flushPara()
  flushList()
  return out
}

export function Markdown({ text }: { text: string }) {
  const parts = text.split(/```/)
  return (
    <div className="md">
      {parts.map((part, i) => {
        if (i % 2 === 0) return <Fragment key={i}>{textBlocks(part, String(i))}</Fragment>
        // First line of a fence is the (optional) language tag.
        const nl = part.indexOf('\n')
        const lang = nl >= 0 ? part.slice(0, nl).trim() : ''
        const code = (nl >= 0 ? part.slice(nl + 1) : part).replace(/\n$/, '')
        return (
          <div key={i} className="md-fence">
            <div className="md-fence-bar">
              <span>{lang || 'text'}</span>
              <CopyButton text={code} />
            </div>
            <pre>{code}</pre>
          </div>
        )
      })}
    </div>
  )
}
