<svelte:window on:keydown={onKey} />

<div class="scrim" on:click|self={() => dispatch('close')} role="dialog" aria-modal="true" aria-label="Core log">
  <div class="sheet logs">
    <header>
      <h3>Core log</h3>
      <span class="sub">what the embedded Xray instance prints — kept in memory only</span>
      <span style="flex:1" />
      <div class="log-filters">
        <button class="filter-chip" class:active={filter === 'all'} on:click={() => (filter = 'all')}>All</button>
        <button class="filter-chip" class:active={filter === 'error'} on:click={() => (filter = 'error')}>Errors</button>
        <button class="filter-chip" class:active={filter === 'warn'} on:click={() => (filter = 'warn')}>Warnings</button>
      </div>
      <label class="btn ghost sm" style="cursor:pointer" title="Auto-scroll on new logs">
        <input type="checkbox" bind:checked={auto} style="width:auto;margin:0 6px 0 0;accent-color:var(--accent)" />live
      </label>
      <button class="btn ghost sm" on:click={copyLogs} title="Copy all logs">
        {@html icons.copy} {copied ? 'Copied!' : 'Copy'}
      </button>
      <button class="btn ghost sm" on:click={() => (lines = [])} title="Clear visible log">Clear</button>
      <button class="btn ghost sm" on:click={load} title="Refresh logs">{@html icons.refresh}</button>
      <button class="iconbtn" on:click={() => dispatch('close')} aria-label="Close">{@html icons.x}</button>
    </header>
    <div class="body" bind:this={bodyEl}>
      {#if filteredLines.length === 0}
        <div style="padding:22px 16px;color:var(--text-faint);font-size:12.5px">
          {#if lines.length === 0}
            Nothing logged yet. Connect a node, or raise the core log level in Settings to “info”.
          {:else}
            No log lines match the selected filter.
          {/if}
        </div>
      {:else}
        {#each filteredLines as l, i (i)}
          <div class="logline {l.level || 'info'}">
            <span class="t">{l.time || ''}</span>
            <span class="l">{l.level || 'info'}</span>
            <span class="text-content">{l.text}</span>
          </div>
        {/each}
      {/if}
    </div>
    <footer>
      <span class="hint" style="margin:0">{filteredLines.length} line{filteredLines.length === 1 ? '' : 's'}</span>
      <span class="grow" />
      <button class="btn" on:click={() => dispatch('close')}>Close</button>
    </footer>
  </div>
</div>

<script>
  import { onMount, onDestroy, createEventDispatcher, tick } from 'svelte';
  import { api, on } from '../lib/bridge.js';
  import { icons } from '../lib/icons.js';
  import { copyText } from '../lib/utils.js';

  const dispatch = createEventDispatcher();
  let lines = [];
  let auto = true;
  let filter = 'all';
  let copied = false;
  let timer;
  let off;
  let bodyEl;

  $: filteredLines = filter === 'all'
    ? lines
    : filter === 'error'
      ? lines.filter((l) => l.level === 'error')
      : lines.filter((l) => l.level === 'warn' || l.level === 'warning');

  async function load() {
    try {
      lines = (await api.getLogs(400)) || [];
      await scrollToBottom();
    } catch (e) { /* closing the sheet mid-read is fine */ }
  }

  async function scrollToBottom() {
    if (!auto) return;
    await tick();
    if (bodyEl) bodyEl.scrollTop = bodyEl.scrollHeight;
  }

  async function copyLogs() {
    const text = filteredLines.map((l) => `[${l.time || ''}] [${(l.level || 'info').toUpperCase()}] ${l.text}`).join('\n');
    const ok = await copyText(text);
    if (ok) {
      copied = true;
      setTimeout(() => (copied = false), 2000);
    }
  }

  function onKey(e) {
    if (e.key === 'Escape') dispatch('close');
  }

  onMount(() => {
    load();
    timer = setInterval(() => { if (auto) load(); }, 1200);
    off = on('log', async (l) => {
      if (!l) return;
      lines = [...lines.slice(-399), l];
      await scrollToBottom();
    });
  });

  onDestroy(() => {
    clearInterval(timer);
    off?.();
  });
</script>
