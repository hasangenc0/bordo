import { initTracing } from './otel.js'

const SERVICE_NAME = process.env.OTEL_SERVICE_NAME ?? '{{.ProjectName}}'
initTracing(SERVICE_NAME)

import Fastify from 'fastify'
import httpProxy from '@fastify/http-proxy'

const PORT = parseInt(process.env.PORT ?? '3000', 10)
const BACKEND_URL = process.env.BACKEND_URL ?? '{{.BackendURL}}'

const app = Fastify({ logger: true })

app.get('/health', async () => ({ status: 'ok', service: '{{.ProjectName}}' }))

app.register(httpProxy, {
  upstream: BACKEND_URL,
  prefix: '/api/v1',
  rewritePrefix: '/api/v1',
})

app.listen({ port: PORT, host: '0.0.0.0' }, (err) => {
  if (err) {
    app.log.error(err)
    process.exit(1)
  }
})
