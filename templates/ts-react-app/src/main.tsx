import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { initTracing } from './otel'

initTracing(import.meta.env.VITE_OTEL_SERVICE_NAME ?? 'app')

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
