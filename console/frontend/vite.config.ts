import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
      // Chat WebSocket, relayed to the agent by the BFF (nginx does this in production).
      '/ws': {
        target: 'ws://localhost:3001',
        ws: true,
      },
    },
  },
})
