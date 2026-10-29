import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  resolve: {
    // The workspace root hoists a different React (the BFF's) next to this app's own. Libraries
    // such as react-markdown live in the root and would otherwise bundle that copy, and elements
    // from two Reacts do not render.
    dedupe: ['react', 'react-dom'],
  },
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
