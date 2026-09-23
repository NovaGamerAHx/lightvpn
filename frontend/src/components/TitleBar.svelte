<svelte:options immutable={false} />

<div class="titlebar">
  <span class="brand" title="{state?.appName} {state?.appVersion}">
    <span class="dot" class:on={state?.connected} />
    LightVPN
  </span>
  {#if isMock}<span class="previewtag">preview · no backend</span>{/if}
  <span class="spacer" />
  {#if state?.current?.name}
    <span class="ver">{state.current.name}</span>
  {/if}
  <span class="ver">{state?.coreVersion ? `xray ${state.coreVersion}` : ''}</span>
  <div class="wbtns">
    <button class="wbtn" on:click|preventDefault={() => dispatch('minimise')} title="Minimise" aria-label="Minimise">
      {@html icons.min}
    </button>
    <button class="wbtn" on:click|preventDefault={() => dispatch('maximise')}
      title={maximised ? 'Restore' : 'Maximise'} aria-label={maximised ? 'Restore window' : 'Maximise window'}>
      {@html maximised ? icons.restore : icons.max}
    </button>
    <button class="wbtn close" on:click|preventDefault={() => dispatch('close')} title="Hide to tray" aria-label="Close">
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
</script>
