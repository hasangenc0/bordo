import type { ChatSession, SidebarSection } from '../types'

interface Props {
  active: SidebarSection
  onSelect: (s: SidebarSection) => void
  chats: ChatSession[]
  activeChatId: string | null
  onChatSelect: (id: string) => void
  onNewChat: () => void
  onDeleteChat: (id: string) => void
  chatsLoading: boolean
}

const NAV_ITEMS: { id: SidebarSection; label: string; icon: string }[] = [
  { id: 'projects', label: 'Projects', icon: '⬡' },
  { id: 'build', label: 'Build', icon: '⚙' },
  { id: 'release', label: 'Release', icon: '🚀' },
  { id: 'observe', label: 'Observe', icon: '◎' },
  { id: 'metrics', label: 'Metrics', icon: '▦' },
]

function truncate(s: string, max: number) {
  return s.length > max ? s.slice(0, max) + '…' : s
}

export function Sidebar({
  active,
  onSelect,
  chats,
  activeChatId,
  onChatSelect,
  onNewChat,
  onDeleteChat,
  chatsLoading,
}: Props) {
  return (
    <nav className="sidebar">
      <div className="sidebar-logo">Bordo</div>

      <button className="new-chat-btn" onClick={onNewChat}>
        <span className="new-chat-icon">+</span>
        New Chat
      </button>

      <div className="chat-list">
        {chatsLoading && chats.length === 0 && (
          <div className="chat-list-empty">Loading…</div>
        )}
        {!chatsLoading && chats.length === 0 && (
          <div className="chat-list-empty">No chats yet</div>
        )}
        {chats.map((chat) => (
          <div
            key={chat.id}
            className={`chat-list-item${activeChatId === chat.id ? ' chat-list-item--active' : ''}`}
            onClick={() => onChatSelect(chat.id)}
          >
            <span className="chat-list-title">{truncate(chat.title, 35)}</span>
            <button
              className="chat-list-delete"
              onClick={(e) => {
                e.stopPropagation()
                onDeleteChat(chat.id)
              }}
              title="Delete chat"
            >
              ✕
            </button>
          </div>
        ))}
      </div>

      <div className="sidebar-bottom">
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
