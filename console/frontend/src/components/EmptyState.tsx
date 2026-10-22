import { Circle } from 'lucide-react'
import type { ReactNode } from 'react'

interface Props {
  icon?: ReactNode
  title: string
  hint?: string
}

export function EmptyState({ icon = <Circle size={26} strokeWidth={1.5} aria-hidden="true" />, title, hint }: Props) {
  return (
    <div className="empty-state">
      <div className="empty-state-icon">{icon}</div>
      <div className="empty-state-title">{title}</div>
      {hint && <div className="empty-state-hint">{hint}</div>}
    </div>
  )
}
