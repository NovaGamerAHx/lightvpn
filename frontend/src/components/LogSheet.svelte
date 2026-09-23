<div class="scrim" on:click|self={() => dispatch('close')} role="dialog" aria-modal="true" aria-label="Core log">
  <div class="sheet logs">
    <header>
      <h3>Core log</h3>
      <span class="sub">what the embedded Xray instance prints — kept in memory only</span>
      <span style="flex:1" />
      <label class="btn ghost sm" style="cursor:pointer">
        <input type="checkbox" bind:checked={auto} style="width:auto;margin:0 6px 0 0;accent-color:var(--accent)" />live
      </label>
      <button class="btn ghost sm" on:click={load}>{@html icons.refresh}</button>
      <button class="iconbtn" on:click={() => dispatch('close')} aria-label="Close">{@html icons.x}</button>
    </header>
    <div class="body">
      {#if lines.length === 0}
        <div style="padding:22px 16px;color:var(--text-faint);font-size:12.5px">
          Nothing logged yet. Connect a node, or raise the core log level in Settings to “info”.
        </div>
      {:else}
        {#each lines as l, i (i)}
          <div class="logline {l.level || 'info'}">
            <span class="t">{l.time || ''}</span>
            <span class="l">{l.level || 'info'}</span>
            <span>{l.text}</span>
          </div>
        {/each}
      {/if}
    </div>
    <footer>
      <span class="hint" style="margin:0">{lines.length} line{lines.length === 1 ? '' : 's'}</span>
      <span class="grow" />
      <button class="btn" on:click={() => dispatch('close')}>Close</button>
    </footer>
  </div>
</div>

<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { api, on } from '../lib/bridge.js';
  import { icons } from '../lib/icons.js';

  const dispatch = createEventDispatcher();
  let lines = [];
  let auto = true;
  let timer;
  let off;

  async function load() {
    try {
      lines = (await api.getLogs(400)) || [];
      await tick();
      const el = document.querySelector('.logs .body');
      if (el) el.scrollTop = el.scrollHeight;
    } catch (e) { /* closing the sheet mid-read is fine */ }
  }

  onMount(() => {
    load();
    timer = setInterval(() => { if (auto) load(); }, 1200);
    off = on('log', (l) => {
      if (!auto) return;
      lines = [...lines.slice(-399), l];
    });
  });

  onDestroy(() => { clearInterval(timer); off?.(); });
</script>
