<div class="scrim" on:click|self={() => dispatch('close')} role="dialog" aria-modal="true" aria-label="Import nodes">
  <div class="sheet">
    <header>
      <h3>Import nodes</h3>
      <span class="sub">one share link per line — paste from V2RayN, a panel, or a subscription file</span>
      <span style="flex:1" />
      <button class="btn ghost sm" on:click={() => (text = '')} title="Clear">Clear</button>
      <button class="iconbtn" on:click={() => dispatch('close')} title="Close (Esc)" aria-label="Close">{@html icons.x}</button>
    </header>

    <div class="body">
      <textarea bind:this={area} bind:value={text} spellcheck="false" placeholder={placeholder} rows="10"></textarea>
      <div class="hint">
        Accepted: <code>vless://</code> and <code>trojan://</code> with <code>type=ws|tcp|grpc</code>,
        <code>security=tls|reality</code>, plus <code>sni</code>, <code>host</code>, <code>path</code>, <code>fp</code>,
        <code>alpn</code>, <code>pbk</code>, <code>sid</code>, <code>spx</code>, <code>flow</code>, <code>headerType</code>,
        <code>allowInsecure</code>. The <code>#name</code> part becomes the row label.
      </div>
      {#if result?.errors?.length}
        <ul class="errlist">
          {#each result.errors as e}
            <li><span class="ln">line {e.line}</span><span><span>{e.error}</span><div class="src">{e.text}</div></span></li>
          {/each}
        </ul>
      {/if}
    </div>

    <footer>
      <span class="hint" style="margin:0">{count} link{count === 1 ? '' : 's'} detected</span>
      <span class="grow" />
      <button class="btn" on:click={() => dispatch('close')}>Cancel</button>
      <button class="btn primary" disabled={count === 0} on:click={submit} autofocus>
        {@html icons.plus} {count ? `Import ${count}` : 'Import'}
      </button>
    </footer>
  </div>
</div>

<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { icons } from '../lib/icons.js';

  export let preset = '';
  const dispatch = createEventDispatcher();

  let text = preset;
  let area;
  let result = null;

  const placeholder = `vless://12345678-1234-1234-1234-123456789012@example.net:443?encryption=none&security=tls&sni=example.net&fp=chrome&type=ws&host=example.net&path=%2Fws#DE%20Frankfurt
12345678-1234-1234-1234-123456789012`;

  $: count = text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => /^(vless|trojan):\/\//i.test(l)).length;

  onMount(() => {
    area?.focus();
    // Pasting is the normal path: accept a clipboard paste of many lines.
    area?.addEventListener('paste', () => setTimeout(() => (text = area.value), 0));
  });

  function submit() {
    dispatch('import', { text });
  }

  function onKey(e) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) submit();
  }
</script>

<svelte:window on:keydown={onKey} />
