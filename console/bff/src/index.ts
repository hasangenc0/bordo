import cors from 'cors'
import express from 'express'
import type { Request, Response } from 'express'
import { createServer } from 'http'
import WebSocket, { WebSocketServer } from 'ws'

const app = express()
app.use(cors())
app.use(express.json())

const AGENT_BASE = process.env.BORDO_AGENT_URL ?? 'http://localhost:7402'
const AGENT_TOKEN = process.env.BORDO_AGENT_TOKEN ?? ''

function agentHeaders(): Record<string, string> {
  if (AGENT_TOKEN) return { Authorization: `Bearer ${AGENT_TOKEN}` }
  return {}
}

async function proxyToAgent(req: Request, res: Response, agentPath: string, method = 'GET', body?: unknown) {
  try {
    const opts: RequestInit = { method, headers: agentHeaders() }
    if (body !== undefined) {
      ;(opts.headers as Record<string, string>)['Content-Type'] = 'application/json'
      opts.body = JSON.stringify(body)
    }
    const r = await fetch(`${AGENT_BASE}${agentPath}`, opts)
    const data = (await r.json()) as unknown
    res.status(r.status).json(data)
  } catch {
    res.status(502).json({ error: 'agent unavailable' })
  }
}

// Chat sessions
app.get('/api/chats', (req, res) => proxyToAgent(req, res, '/chats'))
app.post('/api/chats', (req, res) => proxyToAgent(req, res, '/chats', 'POST'))
app.get('/api/chats/:id', (req, res) => proxyToAgent(req, res, `/chats/${req.params.id}`))
app.delete('/api/chats/:id', (req, res) => proxyToAgent(req, res, `/chats/${req.params.id}`, 'DELETE'))
app.put('/api/chats/:id/title', (req, res) =>
  proxyToAgent(req, res, `/chats/${req.params.id}/title`, 'PUT', req.body),
)

app.get('/api/tools', async (_req, res) => {
  try {
    const r = await fetch(`${AGENT_BASE}/mcp/tools`, { headers: agentHeaders() })
    const data = (await r.json()) as unknown
    res.json(data)
  } catch {
    res.status(502).json({ error: 'agent unavailable' })
  }
})

app.get('/health', (_req, res) => {
  res.json({ status: 'ok' })
})

const PORT = process.env.PORT ?? 3001
const httpServer = createServer(app)

// WebSocket proxy: browser → BFF (no auth needed, on VPN) → agent (auth added here)
const wss = new WebSocketServer({ server: httpServer, path: '/ws/chat' })

wss.on('connection', (clientWs, req) => {
  const chatId = new URL(req.url ?? '', 'ws://localhost').searchParams.get('chat_id') ?? ''
  let agentWsURL = AGENT_BASE.replace(/^http/, 'ws') + '/ws/chat?chat_id=' + chatId
  if (AGENT_TOKEN) agentWsURL += '&token=' + AGENT_TOKEN

  const agentWs = new WebSocket(agentWsURL)

  agentWs.on('message', (data) => {
    if (clientWs.readyState === clientWs.OPEN) clientWs.send(data as Buffer)
  })
  agentWs.on('close', () => clientWs.close())
  agentWs.on('error', () => clientWs.close())

  clientWs.on('message', (data) => {
    if (agentWs.readyState === agentWs.OPEN) agentWs.send(data as Buffer)
  })
  clientWs.on('close', () => agentWs.close())
  clientWs.on('error', () => agentWs.close())
})

httpServer.listen(PORT, () => {
  console.log(`Bordo console BFF listening on :${PORT}`)
})
