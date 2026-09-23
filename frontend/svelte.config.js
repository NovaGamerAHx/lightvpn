import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

export default {
  // Accessibility hints from Svelte's compiler are advisory for a desktop webview
  // (no screen reader, no tab focus on the scrim). Real errors still fail the build.
  onwarn(warning, defaultHandler) {
    if (/A11y:/.test(warning.message || '')) return;
    defaultHandler(warning);
  },
  // Strips nothing fancy: plain Svelte, no TS, no external stores.
  preprocess: vitePreprocess(),
  compilerOptions: { immutable: false },
};
