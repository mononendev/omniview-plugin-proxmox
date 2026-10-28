import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { omniviewExternals } from '@omniviewdev/vite-plugin';
import { injectCssIntoEntry } from './vite/omniview';

// https://vitejs.dev/config/
// Managed dev mode: omniviewExternals() applies the workspace-owned port,
// strictPort and HMR endpoint, so none of them are hardcoded here.
export default defineConfig({
  cacheDir: process.env.OMNIVIEW_VITE_CACHE_DIRECTORY,
  plugins: [react(), omniviewExternals(), injectCssIntoEntry('proxmox')],
  server: {
    host: '127.0.0.1',
    cors: true,
    hmr: {
      protocol: 'ws',
      host: '127.0.0.1',
    },
  },
  build: {
    outDir: process.env.OMNIVIEW_BUILD_OUTPUT_DIRECTORY || 'dist',
    cssCodeSplit: false,
    sourcemap: true,
    rollupOptions: {
      input: 'src/entry.ts',
      output: {
        entryFileNames: 'assets/entry.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/[name].[ext]',
        format: 'system',
      },
      preserveEntrySignatures: 'exports-only',
    },
  },
});
