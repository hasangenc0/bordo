import type { ReactNode } from 'react'

interface Props {
  title: string
  action?: ReactNode
}

export function PageHeader({ title, action }: Props) {
  return (
    <div className="page-header">
      <h1 className="page-title">{title}</h1>
      {action && <div>{action}</div>}
    </div>
  )
}
