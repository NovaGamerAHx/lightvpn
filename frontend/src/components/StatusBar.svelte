<div class="statusbar">
  <span class="chip" title="In-process Xray version — no xray.exe, no subprocess">
    xray-core {state?.coreVersion || '—'}
  </span>
  <span class="chip" class:warn={!!state?.error} title="Local inbounds">
    socks <b>{state?.settings?.socksPort || 10808}</b> · http <b>{state?.settings?.httpPort || 10809}</b>
  </span>
  <button class="chip" class:warn={!state?.proxy?.supported} on:click={() => dispatch('logs')}
    title="{state?.proxy?.key}&#10;Click for the core log">
    system proxy: <b>{proxyLabel}</b>
  </button>
  <span class="grow" />
  {#if state?.connected}<span class="k">up {fmtUptime(state?.elapsedMs || 0)}</span>{/if}
  <button class="chip" on:click={() => dispatch('config')} title="Show the config handed to Xray">
    {@html icons.terminal} config
  </button>
  <span class="k" title="Everything the app stores">{clip(state?.storePath || 'config.json')}</span>
</div>

<script>
  import { createEventDispatcher } from 'svelte';
  import { icons } from '../lib/icons.js';
  import { fmtUptime } from '../lib/utils.js';

  export let state = null;
  const dispatch = createEventDispatcher();

  $: proxyLabel = state?.proxy?.enabledByApp
    ? `on → ${state.proxy.proxyServer}`
    : state?.proxy?.supported === false
      ? 'unavailable'
      : state?.proxy?.proxyEnable
        ? 'set by someone else'
        : 'off';

  function clip(p) {
    const s = String(p || '');
    return s.length > 34 ? '…' + s.slice(-33) : s;
  }
</script>
