<div class="statusbar">
  <span class="chip" title="In-process embedded Xray instance — no subprocess">
    xray-core {state?.coreVersion || '—'}
  </span>
  <span class="chip" class:warn={!!state?.error} title="Local inbound listener ports">
    socks <b>{state?.settings?.socksPort || 10808}</b> · http <b>{state?.settings?.httpPort || 10809}</b>
  </span>
  <button class="chip btn-chip" class:warn={!state?.proxy?.supported} on:click={() => dispatch('logs')}
    title="{state?.proxy?.key || ''}&#10;Click to open core logs">
    system proxy: <b>{proxyLabel}</b>
  </button>
  <span class="grow" />
  {#if state?.connected}
    <span class="k uptime-text">up {fmtUptime(elapsed || state?.elapsedMs || 0)}</span>
  {/if}
  <button class="chip btn-chip" on:click={() => dispatch('logs')} title="Show core log messages">
    {@html icons.terminal} logs
  </button>
  <button class="chip btn-chip" on:click={() => dispatch('config')} title="Show the config handed to Xray">
    {@html icons.eye} config
  </button>
  <span class="k" title="Configuration file path: {state?.storePath || 'config.json'}">
    {clip(state?.storePath || 'config.json')}
  </span>
</div>

<script>
  import { createEventDispatcher } from 'svelte';
  import { icons } from '../lib/icons.js';
  import { fmtUptime } from '../lib/utils.js';

  export let state = null;
  export let elapsed = 0;
  const dispatch = createEventDispatcher();

  $: proxyLabel = state?.proxy?.enabledByApp
    ? `on → ${state.proxy.proxyServer}`
    : state?.proxy?.supported === false
      ? 'unavailable'
      : state?.proxy?.proxyEnable
        ? 'active (external)'
        : 'off';

  function clip(p) {
    const s = String(p || '');
    return s.length > 30 ? '…' + s.slice(-29) : s;
  }
</script>
