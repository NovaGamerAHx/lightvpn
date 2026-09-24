<svelte:window on:keydown={onKey} />

<div class="scrim" on:click|self={() => dispatch('close')} role="dialog" aria-modal="true" aria-label="Import nodes">
  <div class="sheet">
    <header>
      <h3>Import nodes</h3>
      <span class="sub">one share link per line or base64 subscription — paste from V2RayN, a panel, or a subscription file</span>
      <span style="flex:1" />
      <button class="btn ghost sm" on:click={pasteClipboard} title="Paste from clipboard">{@html icons.copy} Paste</button>
      <button class="btn ghost sm" on:click={() => (text = '')} title="Clear">Clear</button>
      <button class="iconbtn" on:click={() => dispatch('close')} title="Close (Esc)" aria-label="Close">{@html icons.x}</button>
    </header>

    <div class="body">
      <textarea bind:this={area} bind:value={text} spellcheck="false" placeholder={placeholder} rows="10"></textarea>
      <div class="hint">
        Accepted: <code>vless://</code> and <code>trojan://</code> links or base64 encoded subscription lists.
        Parameters like <code>type=ws|tcp|grpc</code>, <code>security=tls|reality</code>, <code>sni</code>, <code>host</code>,
        <code>path</code>, <code>fp</code>, <code>alpn</code>, <code>pbk</code>, <code>sid</code>, <code>flow</code> are parsed automatically.
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
      <span class="hint" style="margin:0">
        {#if count > 0}
          {count} link{count === 1 ? '' : 's'} detected
        {:else if isBase64}
          Base64 subscription detected
        {:else if text.trim()}
          Ready to parse
        {:else}
          Paste links or subscription above
        {/if}
      </span>
      <span class="grow" />
      <button class="btn" on:click={() => dispatch('close')}>Cancel</button>
      <button class="btn primary" disabled={!text.trim()} on:click={submit} autofocus>
        {@html icons.plus} {count > 0 ? `Import (${count})` : 'Import'}
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
trojan://password123@example.org:443?security=tls&sni=example.org&type=tcp#FR%20Paris
Or paste a Base64 subscription string directly`;

  $: count = countLinks(text);
  $: isBase64 = count === 0 && text.trim().length > 20 && !text.includes('://') && /^[A-Za-z0-9+/=\r\n\s_-]+$/.test(text.trim());

  function countLinks(src) {
    if (!src) return 0;
    const direct = src
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => /^(vless|trojan):\/\//i.test(l)).length;
    if (direct > 0) return direct;

    // Check if it's base64 encoded
    const clean = src.replace(/[\r\n\s]/g, '');
    if (clean.length > 20 && !clean.includes('://')) {
      try {
        const decoded = atob(clean);
        return decoded
          .split('\n')
          .map((l) => l.trim())
          .filter((l) => /^(vless|trojan):\/\//i.test(l)).length;
      } catch (_) {}
    }
    return 0;
  }

  async function pasteClipboard() {
    try {
      if (navigator.clipboard?.readText) {
        const clip = await navigator.clipboard.readText();
        if (clip) {
          text = clip;
          area?.focus();
        }
      }
    } catch (_) {}
  }

  onMount(() => {
    area?.focus();
    area?.addEventListener('paste', () => setTimeout(() => (text = area.value), 0));
  });

  function submit() {
    if (!text.trim()) return;
    dispatch('import', { text: text.trim() });
  }

  function onKey(e) {
    if (e.key === 'Escape') dispatch('close');
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) submit();
  }
</script>
