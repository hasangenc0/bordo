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

// Mirrors the production nginx: /api/tools -> /mcp/tools, /api/* -> agent root.
// The agent in turn proxies /v1/* to the control plane, so the console can reach
// every control-plane endpoint through this one path.
app.use('/api', async (req: Request, res: Response) => {
  const path = req.url === '/tools' ? '/mcp/tools' : req.url
  const hasBody = !['GET', 'HEAD', 'DELETE'].includes(req.method) && req.body !== undefined
  try {
    const headers: Record<string, string> = { ...agentHeaders() }
    if (hasBody) headers['Content-Type'] = 'application/json'
    const r = await fetch(`${AGENT_BASE}${path}`, {
      method: req.method,
      headers,
      body: hasBody ? JSON.stringify(req.body) : undefined,
    })
    const text = await r.text()
    res.status(r.status)
    const ct = r.headers.get('content-type')
    if (ct) res.setHeader('Content-Type', ct)
    res.send(text)
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

  // Preserve the frame type: the browser expects the agent's text frames as text.
  agentWs.on('message', (data, isBinary) => {
    if (clientWs.readyState === clientWs.OPEN) clientWs.send(data, { binary: isBinary })
  })
  agentWs.on('close', () => clientWs.close())
  agentWs.on('error', () => clientWs.close())

  clientWs.on('message', (data, isBinary) => {
    if (agentWs.readyState === agentWs.OPEN) agentWs.send(data, { binary: isBinary })
  })
  clientWs.on('close', () => agentWs.close())
  clientWs.on('error', () => agentWs.close())
})

httpServer.listen(PORT, () => {
  console.log(`Bordo console BFF listening on :${PORT}`)
})
