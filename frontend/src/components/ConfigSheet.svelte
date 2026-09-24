<svelte:window on:keydown={onKey} />

<div class="scrim" on:click|self={() => dispatch('close')} role="dialog" aria-modal="true" aria-label="Generated config">
  <div class="sheet">
    <header>
      <h3>Generated Xray config</h3>
      <span class="sub">{title || ''}</span>
      <span style="flex:1" />
      <button class="btn ghost sm" on:click={load} title="Regenerate">{@html icons.refresh} Refresh</button>
      <button class="btn ghost sm" on:click={copy} title="Copy JSON">
        {@html icons.copy} {copied ? 'Copied!' : 'Copy'}
      </button>
      <button class="iconbtn" on:click={() => dispatch('close')} aria-label="Close">{@html icons.x}</button>
    </header>
    <div class="body">
      {#if node?.certNote || node?.certPin || node?.notes?.length}
        <div class="trust">
          {#if node.certNote}
            <div class="row">
              <span class="k">Trust</span>
              <span class="v">{node.certNote}</span>
              {#if node.certPin}
                <button class="btn ghost sm" on:click={unpin} title="Forget the pinned certificate — the next connect measures it again">Unpin</button>
              {/if}
            </div>
          {/if}
          {#if node.certPin}
            <div class="row">
              <span class="k">SHA-256</span>
              <code class="v mono">{node.certPin}</code>
            </div>
          {/if}
          {#each node.notes || [] as n}
            <div class="row"><span class="k">Note</span><span class="v">{n}</span></div>
          {/each}
        </div>
      {/if}
      {#if error}
        <ul class="errlist" style="margin-top:0">
          <li><span class="ln">error</span><span>{error}</span></li>
        </ul>
      {:else}
        <pre class="code">{json || '…'}</pre>
        <div class="hint">
          This is the exact JSON handed to the embedded <code>xray-core</code> instance — no subprocess, no other file.
          Inbound ports are loopback-only; the first outbound is the tunnel, so anything not matched by a rule goes through it.
        </div>
      {/if}
    </div>
    <footer>
      <span class="grow" />
      <button class="btn" on:click={() => dispatch('close')}>Close</button>
    </footer>
  </div>
</div>

<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { api } from '../lib/bridge.js';
  import { icons } from '../lib/icons.js';
  import { copyText } from '../lib/utils.js';

  export let id = '';
  export let title = '';
  export let node = null;
  const dispatch = createEventDispatcher();

  let json = '';
  let error = '';
  let copied = false;
  let lastLoadedId = null;

  async function unpin() {
    try {
      await api.clearPin(id);
      dispatch('unpinned');
      await load();
    } catch (e) {
      error = String(e?.message || e);
    }
  }

  async function load() {
    error = '';
    try {
      lastLoadedId = id;
      const raw = await api.preview(id);
      json = typeof raw === 'string' ? pretty(raw) : JSON.stringify(raw, null, 2);
    } catch (e) {
      error = String(e?.message || e);
      json = '';
    }
  }

  function pretty(raw) {
    try { return JSON.stringify(JSON.parse(raw), null, 2); } catch (_) { return raw; }
  }

  async function copy() {
    const ok = await copyText(json || '');
    if (ok) {
      copied = true;
      setTimeout(() => (copied = false), 2000);
    }
  }

  function onKey(e) {
    if (e.key === 'Escape') dispatch('close');
  }

  onMount(load);

  $: if (id && id !== lastLoadedId) {
    load();
  }
</script>
