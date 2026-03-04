// frontend/vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { TanStackRouterVite } from '@tanstack/router-plugin/vite';
import path from 'path';

// In Docker dev, set API_TARGET=http://api:8000 to proxy to the backend service.
// In local dev, defaults to http://localhost:8000.
const apiTarget = process.env.API_TARGET ?? 'http://localhost:8000';

export default defineConfig({
  plugins: [
    TanStackRouterVite({ routesDirectory: './src/routes' }),
    react(),
  ],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    host: true,
    port: 3000,
    proxy: {
      '/api': apiTarget,
      '/static': apiTarget,
    },
  },
  build: {
    outDir: 'dist',
  },
});
