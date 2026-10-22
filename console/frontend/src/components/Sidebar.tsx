import { NavLink } from 'react-router-dom'
import { Activity, Box, Globe, Settings, Sparkles } from 'lucide-react'

const NAV_ITEMS = [
  { to: '/', label: 'Chat', icon: Sparkles, end: true },
  { to: '/apps', label: 'Apps', icon: Box, end: false },
  { to: '/regions', label: 'Regions', icon: Globe, end: false },
  { to: '/observe', label: 'Observe', icon: Activity, end: false },
]

export function Sidebar() {
  return (
    <nav className="sidebar">
      <div className="sidebar-logo">Bordo</div>

      <div className="sidebar-nav-section">
        <ul className="sidebar-nav">
          {NAV_ITEMS.map((item) => (
            <li key={item.to}>
              <NavLink
                to={item.to}
                end={item.end}
                className={({ isActive }: { isActive: boolean }) => `sidebar-item${isActive ? ' active' : ''}`}
              >
                <span className="sidebar-icon"><item.icon size={16} strokeWidth={1.75} aria-hidden="true" /></span>
                {item.label}
              </NavLink>
            </li>
          ))}
        </ul>
      </div>

      <div className="sidebar-bottom">
        <NavLink to="/settings" className={({ isActive }: { isActive: boolean }) => `sidebar-item${isActive ? ' active' : ''}`}>
          <span className="sidebar-icon"><Settings size={16} strokeWidth={1.75} aria-hidden="true" /></span>
          Settings
        </NavLink>
      </div>
    </nav>
  )
}
