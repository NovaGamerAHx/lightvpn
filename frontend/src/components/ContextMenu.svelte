<svelte:window on:click={close} on:contextmenu={close} on:keydown={onKey} on:resize={close} on:scroll={close} capture="true" />

<div class="menu" style="left:{x}px;top:{y}px" role="menu" on:click|stopPropagation on:contextmenu|preventDefault|stopPropagation>
  <div class="head">{clip(menu.name)}</div>
  {#each items as it}
    {#if it === '-'}
      <div class="div" />
    {:else}
      <button class={it.danger ? 'danger' : ''} role="menuitem" on:click={() => act(it.action)}>
        {@html icons[it.icon]}<span>{it.label}</span>
      </button>
    {/if}
  {/each}
</div>

<script>
  import { createEventDispatcher, tick } from 'svelte';
  import { icons } from '../lib/icons.js';

  export let menu;
  const dispatch = createEventDispatcher();

  $: items = menu?.connected
    ? [
        { action: 'disconnect', label: 'Disconnect', icon: 'stop' },
        '-',
        { action: 'preview', label: 'Generated config', icon: 'eye' },
        { action: 'ping', label: 'Test ping', icon: 'ping' },
        { action: 'copy', label: 'Copy share link', icon: 'copy' },
        { action: 'rename', label: 'Rename…', icon: 'edit' },
        '-',
        { action: 'delete', label: 'Delete node', icon: 'trash', danger: true },
      ]
    : [
        { action: 'connect', label: 'Connect', icon: 'play' },
        { action: 'select', label: 'Select', icon: 'check' },
        { action: 'ping', label: 'Test ping', icon: 'ping' },
        { action: 'preview', label: 'Generated config', icon: 'eye' },
        { action: 'copy', label: 'Copy share link', icon: 'copy' },
        { action: 'rename', label: 'Rename…', icon: 'edit' },
        '-',
        { action: 'delete', label: 'Delete node', icon: 'trash', danger: true },
      ];

  $: x = clampX(menu?.x ?? 0);
  $: y = clampY(menu?.y ?? 0);

  function clip(name) {
    const n = String(name || '');
    return n.length > 30 ? n.slice(0, 29) + '…' : n;
  }
  function clampX(v) { return Math.max(8, Math.min(v, (window.innerWidth || 1000) - 200)); }
  function clampY(v) { return Math.max(8, Math.min(v, (window.innerHeight || 700) - 280)); }

  function act(action) {
    dispatch('act', { action, id: menu.id });
  }
  function close() { dispatch('close'); }
  function onKey(e) { if (e.key === 'Escape') close(); }
</script>
