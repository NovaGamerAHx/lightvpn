{#if items?.length}
  <div class="toasts" role="status" aria-live="polite">
    {#each items as t (t.id)}
      <div
        class="toast {t.kind || 'info'}"
        role="button"
        tabindex="0"
        aria-label="Dismiss"
        on:click={() => dispatch('dismiss', t.id)}
        on:keydown={(e) => e.key === 'Enter' && dispatch('dismiss', t.id)}
      >
        <span class="ic">{@html icons[t.kind === 'ok' ? 'check' : t.kind === 'err' ? 'warn' : t.kind === 'warn' ? 'warn' : 'info']}</span>
        <span class="msg">{t.msg}{#if t.detail}<small>{t.detail}</small>{/if}</span>
      </div>
    {/each}
  </div>
{/if}

<script>
  import { createEventDispatcher } from 'svelte';
  import { icons } from '../lib/icons.js';

  export let items = [];
  const dispatch = createEventDispatcher();
</script>
