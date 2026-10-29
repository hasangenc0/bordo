import { isValidElement, type ReactElement, type ReactNode } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { CopyButton } from './CopyButton'

// Chat messages are Markdown: CommonMark plus GitHub's tables, strikethrough, task lists and bare
// URL autolinks. react-markdown builds React nodes (never raw HTML, which it ignores), and only
// links with safe protocols survive its default URL filter.

const heading = (level: 1 | 2 | 3 | 4) => {
  const Heading = ({ children }: { children?: ReactNode }) => (
    <p className={`md-heading md-h${level}`}>{children}</p>
  )
  return Heading
}

// A fenced block arrives as <pre><code class="language-x">…</code></pre>; render it with the
// language bar and copy button instead.
function Fence({ children }: { children?: ReactNode }) {
  const child = Array.isArray(children) ? children[0] : children
  if (!isValidElement(child)) return <pre>{children}</pre>
  const props = (child as ReactElement<{ className?: string; children?: ReactNode }>).props
  const lang = /language-([\w+-]+)/.exec(props.className ?? '')?.[1] ?? ''
  const code = String(props.children ?? '').replace(/\n$/, '')
  return (
    <div className="md-fence">
      <div className="md-fence-bar">
        <span>{lang || 'text'}</span>
        <CopyButton text={code} />
      </div>
      <pre>{code}</pre>
    </div>
  )
}

// Models often emit "| | |" as a header just to get a two-column key/value table; hide that row.
interface HastNode {
  type: string
  value?: string
  children?: HastNode[]
}
const hastText = (n: HastNode): string => (n.value ?? '') + (n.children ?? []).map(hastText).join('')

const components: Components = {
  thead: ({ node, children }) =>
    node && hastText(node as HastNode).trim() === '' ? null : <thead>{children}</thead>,
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noreferrer noopener">
      {children}
    </a>
  ),
  pre: Fence,
  code: ({ children }) => <code className="md-code">{children}</code>, // inline only: Fence handles blocks
  table: ({ children }) => (
    <div className="md-table-wrap">
      <table>{children}</table>
    </div>
  ),
  h1: heading(1),
  h2: heading(2),
  h3: heading(3),
  h4: heading(4),
  h5: heading(4),
  h6: heading(4),
  blockquote: ({ children }) => <blockquote className="md-quote">{children}</blockquote>,
  hr: () => <hr className="md-rule" />,
}

const plugins = [remarkGfm]

export function Markdown({ text }: { text: string }) {
  return (
    <div className="md">
      <ReactMarkdown remarkPlugins={plugins} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  )
}
