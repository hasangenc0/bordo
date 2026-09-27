export interface Message {
  id: string
  role: 'user' | 'assistant'
  content: string
  toolResult?: ToolResult
  timestamp: number
}

export interface ToolResult {
  toolName: string
  data: unknown
}

export type SidebarSection = 'projects' | 'build' | 'release' | 'observe' | 'metrics' | 'settings'

export interface WSIncoming {
  type: 'message' | 'tool_result' | 'error' | 'history'
  content: string
  tool_name?: string
  data?: unknown
  messages?: Array<{ role: 'user' | 'assistant'; content: string }>
}

export interface WSOutgoing {
  type: 'message'
  content: string
  token: string
}

export interface ChatSession {
  id: string
  title: string
  created_at: string
  updated_at: string
}
