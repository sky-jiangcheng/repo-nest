import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // Relative base is required for Wails desktop builds: the frontend runs under
  // a non-HTTP origin (wails:// / file://), so absolute /assets/ paths fail to
  // resolve and dynamic imports throw "The string did not match the expected pattern".
  base: './',
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:18765',
        changeOrigin: true,
      },
    },
    allowedHosts: ['.monkeycode-ai.online'],
  },
  build: {
    outDir: 'dist',
    // Let the npm build script wipe dist; Vite's built-in emptyOutDir triggers
    // bulk-delete guards in this environment.
    emptyOutDir: false,
    // The only remaining chunk above the 500 kB default is mermaid's shared
    // diagram engine (~660 kB). Mermaid is imported on demand in
    // utils/markdown.ts, so it never sits on the initial-load path and cannot
    // be split further upstream.
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      output: {
        // Ensure each page becomes its own chunk so lazy-loaded navigation
        // only fetches the code needed for the destination route.
        manualChunks(id) {
          if (id.includes('src/pages/Home')) return 'home'
          if (id.includes('src/pages/Dashboard')) return 'dashboard'
          if (id.includes('src/pages/Knowledge')) return 'knowledge'
          if (id.includes('src/pages/ProjectDetail')) return 'projectDetail'
          if (id.includes('src/pages/Settings')) return 'settings'
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
    environmentOptions: {
      jsdom: {
        url: 'http://localhost',
      },
    },
    env: {
      NODE_ENV: 'development',
    },
  },
})
