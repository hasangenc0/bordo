import { NavLink } from 'react-router-dom'

const NAV_ITEMS = [
  { to: '/', label: 'Chat', icon: '✦', end: true },
  { to: '/apps', label: 'Apps', icon: '○', end: false },
  { to: '/regions', label: 'Regions', icon: '◎', end: false },
  { to: '/observe', label: 'Observe', icon: '∿', end: false },
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
                <span className="sidebar-icon">{item.icon}</span>
                {item.label}
              </NavLink>
            </li>
          ))}
        </ul>
      </div>

      <div className="sidebar-bottom">
        <NavLink to="/settings" className={({ isActive }: { isActive: boolean }) => `sidebar-item${isActive ? ' active' : ''}`}>
          <span className="sidebar-icon">⚙</span>
          Settings
        </NavLink>
      </div>
    </nav>
  )
}
