interface Props {
  icon?: string
  title: string
  hint?: string
}

export function EmptyState({ icon = '○', title, hint }: Props) {
  return (
    <div className="empty-state">
      <div className="empty-state-icon">{icon}</div>
      <div className="empty-state-title">{title}</div>
      {hint && <div className="empty-state-hint">{hint}</div>}
    </div>
  )
}
