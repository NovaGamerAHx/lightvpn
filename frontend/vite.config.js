import { defineConfig, loadEnv } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import path from 'path';

// Wails serves whatever `npm run build` puts in frontend/dist, embedded by go:embed.
// `base: './'` matters: the asset server mounts the app at wails:///, so absolute
// "/assets/…" URLs would miss.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const port = parseInt(env.VITE_PORT || process.env.WAILS_DEV_PORT || '34115', 10);
  return {
    base: './',
    plugins: [svelte()],
    resolve: {
      alias: {
        $components: path.resolve('./src/components'),
        $lib: path.resolve('./src/lib'),
        $wailsjs: path.resolve('./wailsjs'),
      },
    },
    define: {
      // Set VITE_LIGHTVPN_MOCK=1 to develop the UI with no Go backend at all.
      'process.env.NODE_ENV': JSON.stringify(mode),
    },
    server: {
      port,
      strictPort: true,
      hmr: {
        host: 'localhost',
        protocol: env.WAILS_HMR_PROTOCOL || 'ws',
        port: parseInt(env.VITE_WAILS_HMR_PORT || '0', 10) || undefined,
      },
      fs: { allow: ['./', '../'] },
      watch: { usePolling: !!env.VITE_POLL },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
      target: 'chrome109', // WebView2 rings shipped with Windows 10 LTSC builds
      sourcemap: false,
      cssCodeSplit: false,
      assetsInlineLimit: 2048,
      chunkSizeWarningLimit: 900,
    },
  };
});
