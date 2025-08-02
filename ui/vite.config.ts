import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/strategy': 'http://localhost:3000',
      '/price': 'http://localhost:3000',
      '/history': 'http://localhost:3000',
      '/securities': 'http://localhost:3000',
    }
  }
})
