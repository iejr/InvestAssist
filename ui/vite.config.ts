import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// All backend routes are served by strategy-server-go on :3000.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/bases': 'http://localhost:3000',
      '/quotes': 'http://localhost:3000',
      '/strategies': 'http://localhost:3000',
      '/transactions': 'http://localhost:3000',
    },
  },
})
