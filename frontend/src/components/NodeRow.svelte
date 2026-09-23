<div
  class="node"
  class:sel={node.selected}
  class:live={node.connected}
  class:menu-open={menuOpen}
  role="button"
  tabindex="0"
  on:click={() => dispatch('select', node.id)}
  on:dblclick={() => dispatch('connect', node.id)}
  on:keydown={(e) => {
    if (e.key === 'Enter') dispatch('connect', node.id);
    if (e.key === ' ') { e.preventDefault(); dispatch('select', node.id); }
  }}
  on:contextmenu={(e) => {
    e.preventDefault();
    dispatch('menu', { x: e.clientX, y: e.clientY, id: node.id, name: node.name, connected: node.connected, pinging: node.pinging });
  }}
  title="{node.endpoint}{node.path ? `  ${node.path}` : ''}"
>
  <span class="badge" class:trojan={node.protocol === 'trojan'}>{node.protocol === 'trojan' ? 'Trojan' : 'VLESS'}</span>

  <span style="min-width:0">
    <span class="name">{node.name}</span>
    <span class="meta">
      <span class="t">{node.endpoint}</span>
      <span>·</span>
      <span class="t">{node.transport}</span>
      {#if node.flow}<span>·</span><span class="flagflow">vision</span>{/if}
      {#if node.connected}<span>·</span><span style="color:var(--ok)">active</span>{/if}
      {#if node.pingError}<span>·</span><span style="color:var(--bad);opacity:.85">{node.pingError}</span>{/if}
    </span>
  </span>

  <span class="lat" class:good={cls === 'good'} class:mid={cls === 'mid'} class:bad={cls === 'bad' && !node.pinging}>
    <span class="bars"><i /><i /><i /><i /></span>
    <span class="num" class:pending={!latency}>{latency}</span>
  </span>

  <span class="rowbtns">
    <button class="iconbtn" class:busy={node.pinging || pingRunning} title="Test ping" aria-label="Test ping"
      on:click|stopPropagation|preventDefault={() => dispatch('ping', node.id)}>{@html icons.ping}</button>
    <button class="iconbtn" title="Copy share link" aria-label="Copy link"
      on:click|stopPropagation|preventDefault={() => dispatch('copy', node.id)}>{@html icons.copy}</button>
    <button class="iconbtn" title="Rename" aria-label="Rename"
      on:click|stopPropagation|preventDefault={() => dispatch('rename', node.id)}>{@html icons.edit}</button>
    <button class="iconbtn" title="Show generated Xray config" aria-label="Config"
      on:click|stopPropagation|preventDefault={() => dispatch('preview', node.id)}>{@html icons.eye}</button>
    <button class="iconbtn" title="More actions" aria-label="More"
      on:click|stopPropagation|preventDefault={(e) => dispatch('menu', { x: e.clientX, y: e.clientY - 6, id: node.id, name: node.name, connected: node.connected })}>
      {@html icons.dots}
    </button>
    <button class="iconbtn danger" title="Delete node" aria-label="Delete"
      on:click|stopPropagation|preventDefault={() => dispatch('delete', node.id)}>{@html icons.trash}</button>
  </span>
</div>

<script>
  import { createEventDispatcher } from 'svelte';
  import { icons } from '../lib/icons.js';
  import { pingClass } from '../lib/utils.js';

  export let node;
  export let pingRunning = false;
  export let menuOpen = false;
  const dispatch = createEventDispatcher();

  $: cls = node.pinging || pingRunning ? 'pending' : node.pingMs > 0 ? pingClass(node.pingMs) : node.pingError ? 'bad' : 'unknown';
  $: latency = node.pinging
    ? '···'
    : node.pingMs > 0
      ? `${node.pingMs.toFixed(node.pingMs < 100 ? 1 : 0)} ms`
      : node.pingError
        ? 'fail'
        : '—';
</script>
