<svelte:options immutable={false} />

<div class="titlebar">
  <span class="brand" title="{state?.appName || 'LightVPN'} {state?.appVersion || '1.0.0'}">
    <img src="appicon.png" alt="" width="18" height="18" on:error={(e) => (e.target.style.display = 'none')} />
    <span class="dot" class:on={state?.connected} />
    LightVPN
  </span>
  {#if isMock}<span class="previewtag">preview mode</span>{/if}
  <span class="spacer" />
  {#if state?.current?.name && state?.connected}
    <span class="active-badge" title="Active node: {state.current.name}">{clip(state.current.name)}</span>
  {/if}
  <span class="ver">{state?.coreVersion ? `xray-core ${state.coreVersion}` : ''}</span>
  <div class="wbtns">
    <button class="wbtn" on:click|preventDefault={() => dispatch('minimise')} title="Minimise" aria-label="Minimise">
      {@html icons.min}
    </button>
    <button class="wbtn" on:click|preventDefault={() => dispatch('maximise')}
      title={maximised ? 'Restore' : 'Maximise'} aria-label={maximised ? 'Restore window' : 'Maximise window'}>
      {@html maximised ? icons.restore : icons.max}
    </button>
    <button class="wbtn close" on:click|preventDefault={() => dispatch('close')} title="Close (minimize to tray)" aria-label="Close">
      {@html icons.x}
    </button>
  </div>
</div>

<script>
  import { createEventDispatcher } from 'svelte';
  import { icons } from '../lib/icons.js';
  import { isMock } from '../lib/bridge.js';

  export let state = null;
  export let maximised = false;
  const dispatch = createEventDispatcher();

  function clip(s) {
    if (!s) return '';
    return s.length > 24 ? s.slice(0, 23) + '…' : s;
  }
</script>
