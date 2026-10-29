import { Fragment, type ReactNode } from 'react'
import { CopyButton } from './CopyButton'

// Small, dependency-free renderer for the Markdown models actually emit: fenced code, headings,
// (nested) lists, tables, blockquotes, rules, paragraphs, and inline **bold**, *italic*, `code`,
// [links](https://…) and bare https:// links. Everything is rendered through React nodes, never as
// raw HTML, and only http(s) URLs become links.

// One alternation, tried left to right at each position. Groups: 1 code, 2 bold, 3 markdown link,
// 4 <autolink>, 5 bare URL, 6 italic.
const TOKEN =
  /(`[^`\n]+`)|(\*\*(?:[^*\n]|\*(?!\*))+?\*\*)|(\[[^\]\n]+\]\(https?:\/\/[^)\s]+\))|(<https?:\/\/[^>\s]+>)|(https?:\/\/[^\s<>()[\]]*[^\s<>()[\].,;:!?'"*`])|(\*[^\s*](?:[^*\n]*[^\s*])?\*)/g

function Link({ href, children }: { href: string; children: ReactNode }) {
  return (
    <a href={href} target="_blank" rel="noreferrer noopener">
      {children}
    </a>
  )
}

function inline(text: string, depth = 0): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let n = 0
  for (const m of text.matchAll(TOKEN)) {
    const at = m.index ?? 0
    if (at > last) out.push(<Fragment key={n++}>{text.slice(last, at)}</Fragment>)
    last = at + m[0].length
    if (m[1]) {
      out.push(<code key={n++} className="md-code">{m[1].slice(1, -1)}</code>)
    } else if (m[2]) {
      // Bold may itself hold code, links or italics: **`name` is gone**.
      out.push(<strong key={n++}>{depth < 4 ? inline(m[2].slice(2, -2), depth + 1) : m[2].slice(2, -2)}</strong>)
    } else if (m[3]) {
      const link = /^\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)$/.exec(m[3])!
      out.push(<Link key={n++} href={link[2]}>{depth < 4 ? inline(link[1], depth + 1) : link[1]}</Link>)
    } else if (m[4]) {
      const url = m[4].slice(1, -1)
      out.push(<Link key={n++} href={url}>{url}</Link>)
    } else if (m[5]) {
      out.push(<Link key={n++} href={m[5]}>{m[5]}</Link>)
    } else if (m[6]) {
      out.push(<em key={n++}>{depth < 4 ? inline(m[6].slice(1, -1), depth + 1) : m[6].slice(1, -1)}</em>)
    }
  }
  if (last < text.length) out.push(<Fragment key={n++}>{text.slice(last)}</Fragment>)
  return out
}

// ── Tables ──────────────────────────────────────────────────────────────────

function splitRow(line: string): string[] {
  let t = line.trim()
  if (t.startsWith('|')) t = t.slice(1)
  if (t.endsWith('|') && !t.endsWith('\\|')) t = t.slice(0, -1)
  // Split on unescaped pipes, but not inside `code`.
  const cells: string[] = []
  let cur = ''
  let inCode = false
  for (let i = 0; i < t.length; i++) {
    const c = t[i]
    if (c === '`') inCode = !inCode
    if (c === '\\' && t[i + 1] === '|') {
      cur += '|'
      i++
    } else if (c === '|' && !inCode) {
      cells.push(cur.trim())
      cur = ''
    } else {
      cur += c
    }
  }
  cells.push(cur.trim())
  return cells
}

const SEPARATOR = /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/

function isTableStart(lines: string[], i: number): boolean {
  return lines[i].includes('|') && i + 1 < lines.length && lines[i + 1].includes('-') && SEPARATOR.test(lines[i + 1])
}

type Align = 'left' | 'right' | 'center' | undefined

function alignOf(sep: string): Align {
  const s = sep.trim()
  if (s.startsWith(':') && s.endsWith(':')) return 'center'
  if (s.endsWith(':')) return 'right'
  if (s.startsWith(':')) return 'left'
  return undefined
}

// ── Lists ───────────────────────────────────────────────────────────────────

interface ListNode {
  ordered: boolean
  items: { text: string; children: ListNode[] }[]
}

const LIST_LINE = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/

function buildList(lines: { indent: number; ordered: boolean; text: string }[]): ListNode {
  const root: ListNode = { ordered: lines[0].ordered, items: [] }
  const stack: { indent: number; node: ListNode }[] = [{ indent: lines[0].indent, node: root }]
  for (const l of lines) {
    while (stack.length > 1 && l.indent < stack[stack.length - 1].indent) stack.pop()
    let top = stack[stack.length - 1]
    if (l.indent > top.indent) {
      const parent = top.node.items[top.node.items.length - 1]
      if (parent) {
        const child: ListNode = { ordered: l.ordered, items: [] }
        parent.children.push(child)
        top = { indent: l.indent, node: child }
        stack.push(top)
      }
    }
    top.node.items.push({ text: l.text, children: [] })
  }
  return root
}

function renderList(node: ListNode, key: string): ReactNode {
  const Tag = node.ordered ? 'ol' : 'ul'
  return (
    <Tag key={key}>
      {node.items.map((it, i) => (
        <li key={i}>
          {inline(it.text)}
          {it.children.map((c, j) => renderList(c, `${key}-${i}-${j}`))}
        </li>
      ))}
    </Tag>
  )
}

// ── Blocks ──────────────────────────────────────────────────────────────────

function textBlocks(src: string, keyBase: string): ReactNode[] {
  const out: ReactNode[] = []
  const lines = src.split('\n')
  let para: string[] = []

  const flushPara = () => {
    if (para.length) {
      out.push(<p key={`${keyBase}-p${out.length}`}>{inline(para.join(' '))}</p>)
      para = []
    }
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    if (isTableStart(lines, i)) {
      flushPara()
      const head = splitRow(line)
      const aligns = splitRow(lines[i + 1]).map(alignOf)
      const rows: string[][] = []
      i += 2
      while (i < lines.length && lines[i].trim() !== '' && lines[i].includes('|')) {
        rows.push(splitRow(lines[i]))
        i++
      }
      i-- // the for loop advances
      out.push(
        <div key={`${keyBase}-t${out.length}`} className="md-table-wrap">
          <table>
            {head.some((h) => h !== '') && (
              <thead>
                <tr>
                  {head.map((h, c) => <th key={c} style={{ textAlign: aligns[c] }}>{inline(h)}</th>)}
                </tr>
              </thead>
            )}
            <tbody>
              {rows.map((r, ri) => (
                <tr key={ri}>
                  {head.map((_, c) => <td key={c} style={{ textAlign: aligns[c] }}>{inline(r[c] ?? '')}</td>)}
                </tr>
              ))}
            </tbody>
          </table>
        </div>,
      )
      continue
    }

    const listLine = LIST_LINE.exec(line)
    if (listLine) {
      flushPara()
      const items: { indent: number; ordered: boolean; text: string }[] = []
      while (i < lines.length) {
        const m = LIST_LINE.exec(lines[i])
        if (!m) break
        const indent = m[1].replace(/\t/g, '    ').length
        const ordered = /\d/.test(m[2])
        // A top-level switch between bullets and numbers starts a new list.
        if (items.length && indent <= items[0].indent && ordered !== items[0].ordered) break
        items.push({ indent, ordered, text: m[3] })
        i++
      }
      i--
      out.push(renderList(buildList(items), `${keyBase}-l${out.length}`))
      continue
    }

    const heading = /^(#{1,6})\s+(.*?)\s*#*\s*$/.exec(line)
    if (heading) {
      flushPara()
      out.push(
        <p key={`${keyBase}-h${out.length}`} className={`md-heading md-h${Math.min(heading[1].length, 4)}`}>
          {inline(heading[2])}
        </p>,
      )
      continue
    }

    if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) {
      flushPara()
      out.push(<hr key={`${keyBase}-r${out.length}`} className="md-rule" />)
      continue
    }

    if (/^\s*>/.test(line)) {
      flushPara()
      const quoted: string[] = []
      while (i < lines.length && /^\s*>/.test(lines[i])) {
        quoted.push(lines[i].replace(/^\s*>\s?/, ''))
        i++
      }
      i--
      out.push(
        <blockquote key={`${keyBase}-q${out.length}`} className="md-quote">
          {textBlocks(quoted.join('\n'), `${keyBase}-q${out.length}`)}
        </blockquote>,
      )
      continue
    }

    if (line.trim() === '') {
      flushPara()
    } else {
      para.push(line.trim())
    }
  }
  flushPara()
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
