<div class="hero">
  <div style="min-width:0">
    <span class="state-pill" class:on={connected} class:busy={busy || connecting} class:off={!connected}>
      <span class="led" />
      {connected ? 'Protected' : busy || connecting ? (connecting ? 'Connecting…' : 'Working…') : 'Not connected'}
    </span>

    <h1 title={title}>{title}</h1>

    <div class="sub">
      {#if node}
        <span class="mono">{node.address}:{node.port}</span>
        <span class="sep" />
        <span>{node.protocol === 'trojan' ? 'Trojan' : 'VLESS'} · {node.transport}</span>
        {#if node.flow}<span class="sep" /><span style="color:var(--accent-2)">{node.flow}</span>{/if}
        <span class="sep" />
      {/if}
      <span>SOCKS5 <b class="mono">127.0.0.1:{ports.socks}</b></span>
      <span class="sep" />
      <span>HTTP <b class="mono">127.0.0.1:{ports.http}</b></span>
      {#if state?.proxy?.enabledByApp}
        <span class="sep" />
        <span style="color:#9ff0c4">system proxy on</span>
      {/if}
    </div>

    {#if state?.error}
      <div class="sub" style="margin-top:8px;color:#ffbdbd">⚠ {state.error}</div>
    {:else if state?.warnings?.length}
      <div class="sub" style="margin-top:8px;color:var(--warn)">⚠ {state.warnings[state.warnings.length - 1]}</div>
    {/if}
  </div>

  <button
    class="connect"
    class:stop={connected}
    class:off={!connected}
    disabled={busy}
    on:click={() => dispatch('toggle')}
    title="{connected ? 'Disconnect and restore the system proxy' : 'Connect with the selected node'} (Ctrl+Enter)"
  >
    <svg class="ring" viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="9.2" />
      {#if busy}<path class="spin" d="M12 2.8a9.2 9.2 0 019.2 9.2" />{:else}<path d="M12 6.6v5.6" />{/if}
    </svg>
    {connected ? 'Disconnect' : busy ? 'Working…' : 'Connect'}
    {#if connected && elapsed > 0}<span class="uptime">{fmtUptime(elapsed)}</span>{/if}
  </button>
</div>

<script>
  import { createEventDispatcher } from 'svelte';
  import { fmtUptime } from '../lib/utils.js';

  export let state = null;
  export let selected = null;
  export let busy = false;
  export let elapsed = 0;
  const dispatch = createEventDispatcher();

  $: connected = !!state?.connected;
  $: connecting = !!state?.connecting;
  $: node = connected ? state?.current : selected;
  $: title = connected
    ? state?.current?.name || 'Connected'
    : selected
      ? selected.name
      : 'Nothing imported yet';
  $: ports = {
    socks: state?.settings?.socksPort || 10808,
    http: state?.settings?.httpPort || 10809,
  };
</script>
