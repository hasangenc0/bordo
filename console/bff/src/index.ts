import cors from 'cors'
import express from 'express'
import type { Request, Response } from 'express'

const app = express()
app.use(cors())
app.use(express.json())

const AGENT_BASE = process.env.BORDO_AGENT_URL ?? 'http://localhost:7402'

async function proxyToAgent(req: Request, res: Response, agentPath: string, method = 'GET', body?: unknown) {
  try {
    const opts: RequestInit = { method }
    if (body !== undefined) {
      opts.headers = { 'Content-Type': 'application/json' }
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
    const r = await fetch(`${AGENT_BASE}/mcp/tools`)
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
app.listen(PORT, () => {
  console.log(`Bordo console BFF listening on :${PORT}`)
})
