import type { SidebarSection } from '../types'

interface Props {
  active: SidebarSection
  onSelect: (s: SidebarSection) => void
}

const NAV_ITEMS: { id: SidebarSection; label: string; icon: string }[] = [
  { id: 'projects', label: 'Projects', icon: '⬡' },
  { id: 'build', label: 'Build', icon: '⚙' },
  { id: 'release', label: 'Release', icon: '🚀' },
  { id: 'observe', label: 'Observe', icon: '◎' },
  { id: 'metrics', label: 'Metrics', icon: '▦' },
]

export function Sidebar({ active, onSelect }: Props) {
  return (
    <nav className="sidebar">
      <div className="sidebar-logo">Bordo</div>
      <ul className="sidebar-nav">
        {NAV_ITEMS.map((item) => (
          <li key={item.id}>
            <button
              className={`sidebar-item${active === item.id ? ' active' : ''}`}
              onClick={() => onSelect(item.id)}
            >
              <span className="sidebar-icon">{item.icon}</span>
              {item.label}
            </button>
          </li>
        ))}
      </ul>
      <div className="sidebar-bottom">
        <button
          className={`sidebar-item${active === 'settings' ? ' active' : ''}`}
          onClick={() => onSelect('settings')}
        >
          <span className="sidebar-icon">⚙</span>
          Settings
        </button>
      </div>
    </nav>
  )
}
