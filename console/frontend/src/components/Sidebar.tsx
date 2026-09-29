import { NavLink } from 'react-router-dom'

const NAV_ITEMS = [
  { to: '/services', label: 'Services', icon: '○' },
  { to: '/build', label: 'Build', icon: '⚙' },
  { to: '/release', label: 'Release', icon: '↑' },
  { to: '/observe', label: 'Observe', icon: '◎' },
  { to: '/templates', label: 'Templates', icon: '▣' },
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
                className={({ isActive }) =>
                  `sidebar-item${isActive ? ' active' : ''}`
                }
              >
                <span className="sidebar-icon">{item.icon}</span>
                {item.label}
              </NavLink>
            </li>
          ))}
        </ul>
      </div>

      <div className="sidebar-bottom">
        <NavLink
          to="/settings"
          className={({ isActive }) =>
            `sidebar-item${isActive ? ' active' : ''}`
          }
        >
          <span className="sidebar-icon">⚙</span>
          Settings
        </NavLink>
      </div>
    </nav>
  )
}
