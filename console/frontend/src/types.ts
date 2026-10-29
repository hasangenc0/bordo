export type ToolStatus = 'running' | 'done' | 'error'

export interface ToolActivity {
  name: string
  input?: Record<string, unknown>
  status: ToolStatus
  data?: unknown
  error?: string
}

export interface Message {
  id: string
  role: 'user' | 'assistant'
  content: string
  isError?: boolean
  tool?: ToolActivity
  timestamp: number
}

export interface WSIncoming {
  type: 'message' | 'tool_call' | 'tool_result' | 'done' | 'error' | 'history' | 'action' | 'running'
  content?: string
  role?: 'user' | 'assistant'
  tool_name?: string
  input?: Record<string, unknown>
  data?: unknown
  error?: string
  messages?: Array<{ role: 'user' | 'assistant'; content: string }>
  // action event fields
  action_id?: string
  action_kind?: string // 'deploy' | 'delete' | 'restart' | 'build' | 'rollback'
  action_target?: string
  action_status?: string // 'auto_approved' | 'approved' | 'pending' | 'failed'
  action_ts?: number
  action_log_url?: string
}

export interface WSOutgoing {
  type: 'message' | 'stop'
  content?: string
  token: string
}

export interface ChatSession {
  id: string
  title: string
  created_at: string
  updated_at: string
}

export interface Project {
  id: string
  name: string
  template: string
  git_repo_url: string
  status: string
  release_status?: string // 'running' | 'deploying' | 'failed' | 'unknown'
  release_region?: string
  updated_at: string
}

export interface Release {
  id: string
  project_id: string
  image_tag: string
  region: string
  strategy: string
  status: string
  release_group_id: string
  created_at: string
  updated_at: string
}

export interface Region {
  id: string
  name: string
  status: string
  created_at: string
}

export interface NodeHealth {
  name: string
  ready: boolean
  last_heartbeat?: string
}

export interface Build {
  id: string
  project_id: string
  status: string
  image_name: string
  image_tag: string
  error_message: string
  created_at: string
  completed_at?: string
}

export interface Template {
  name: string
  runtime?: string
  category?: string
  description: string
}

export interface EnvVar {
  name: string
  value: string
  secret: boolean
}

export interface ActionEvent {
  id: string
  kind: string
  target: string
  status: 'auto_approved' | 'approved' | 'pending' | 'failed'
  ts: number
  log_url?: string
}
