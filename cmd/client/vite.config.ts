import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  // npm run dev serves the client, and the API is web_client's: make run-web_client.
  server: {
    proxy: {
      '/api': 'http://localhost:8080'
    }
  }
})
