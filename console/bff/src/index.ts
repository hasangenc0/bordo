import cors from 'cors'
import express from 'express'

const app = express()
app.use(cors())
app.use(express.json())

const AGENT_BASE = process.env.BORDO_AGENT_URL ?? 'http://localhost:7402'

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
