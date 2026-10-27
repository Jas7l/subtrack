import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// In development the API is proxied to the Go server, so the client uses the
// relative base /api/v1 and needs no CORS.
const api = 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': api,
      '/healthz': api,
    },
  },
})
