import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: { outDir: '../webserver/assets/dist', emptyOutDir: true },
  server: { proxy: { '/api': 'http://127.0.0.1:8080' } },
})
