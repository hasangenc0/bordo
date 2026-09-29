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

export type SidebarSection = 'services' | 'build' | 'release' | 'observe' | 'settings'

export interface WSIncoming {
  type: 'message' | 'tool_result' | 'error' | 'history' | 'action'
  content: string
  tool_name?: string
  data?: unknown
  messages?: Array<{ role: 'user' | 'assistant'; content: string }>
  // action event fields
  action_id?: string
  action_kind?: string   // 'deploy' | 'delete' | 'restart' | 'build' | 'rollback'
  action_target?: string
  action_status?: string // 'auto_approved' | 'approved' | 'pending' | 'failed'
  action_ts?: number
  action_log_url?: string
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

// New types for the dashboard

export interface Project {
  id: string
  name: string
  template: string
  git_repo_url: string
  status: string
  release_status?: string  // 'running' | 'deploying' | 'failed' | 'unknown'
  release_region?: string
  updated_at: string
}

export interface Release {
  id: string
  project_id: string
  image_tag: string
  region: string
  status: string
  release_group_id: string
  log: string
  created_at: string
  updated_at: string
}

export interface Environment {
  id: string
  project_name: string
  name: string
  branch: string
  region_name: string
}

export interface Template {
  name: string
  description: string
  category: string
  icon: string
  author: string
  deploy_count: number
  variables: { name: string; description: string; default_val: string; secret: boolean }[]
}

export interface ActionEvent {
  id: string
  kind: string
  target: string
  status: 'auto_approved' | 'approved' | 'pending' | 'failed'
  ts: number
  log_url?: string
}
