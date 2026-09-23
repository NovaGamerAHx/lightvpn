<svelte:window on:keydown={onKey} />

<div class="app">
  <TitleBar {state} {maximised} on:minimise={minimise} on:maximise={maximise} on:close={onClose} />

  <StatusHero
    {state}
    {selected}
    busy={busy}
    elapsed={elapsed}
    on:toggle={toggleConnect}
  />

  <div class="toolbar">
    <span class="count">
      {nodes.length} node{nodes.length === 1 ? '' : 's'}
      {#if pingRunning}· pinging…{:else if lastPingAt}· pinged {new Date(lastPingAt).toLocaleTimeString()}{/if}
    </span>

    <button class="btn" on:click={pingAll} disabled={pingRunning || nodes.length === 0} title="Measure TCP latency to every server">
      {@html icons.ping} {pingRunning ? 'Testing…' : 'Test ping'}
    </button>
    <button class="btn" on:click={() => (sheet = 'import')} title="Paste one or many share links">
      {@html icons.plus} Import
    </button>
    <button class="btn ghost" on:click={() => (sheet = 'settings')} title="Settings">
      {@html icons.gear} Settings
    </button>
  </div>

  <div class="list">
    {#if nodes.length === 0}
      <div class="empty">
        {@html icons.shield}
        <h2>No nodes yet</h2>
        <p>Paste one or many <code>vless://</code> / <code>trojan://</code> share links — V2RayN export format, one per line. Nothing leaves this machine except through the tunnel you start.</p>
        <button class="btn primary" on:click={() => (sheet = 'import')}>{@html icons.plus} Import links</button>
      </div>
    {:else}
      <div class="group">Nodes · click a row to select, right-click for actions</div>
      {#each nodes as n (n.id)}
        <NodeRow
          node={n}
          pingRunning={pingRunning}
          menuOpen={menu?.id === n.id}
          on:select={(e) => select(e.detail)}
          on:ping={(e) => pingOne(e.detail)}
          on:connect={(e) => connect(e.detail)}
          on:copy={(e) => copyLink(e.detail)}
          on:rename={(e) => rename(e.detail)}
          on:delete={(e) => remove(e.detail)}
          on:preview={(e) => { previewId = e.detail; sheet = 'config'; }}
          on:menu={(e) => (menu = e.detail)}
        />
      {/each}
    {/if}
  </div>

  <StatusBar {state} on:logs={() => (sheet = 'logs')} on:config={() => { previewId = selected?.id || ''; sheet = 'config'; }} />

  {#if menu}
    <ContextMenu {menu} on:act={onMenuAction} on:close={() => (menu = null)} />
  {/if}

  {#if sheet === 'import'}
    <ImportSheet on:close={() => (sheet = null)} on:import={doImport} />
  {:else if sheet === 'settings'}
    <SettingsSheet settings={state?.settings} proxy={state?.proxy} on:close={() => (sheet = null)} on:save={saveSettings} />
  {:else if sheet === 'config'}
    <ConfigSheet
      id={previewId}
      title={(nodes.find((n) => n.id === previewId) || selected)?.name || 'Generated config'}
      node={nodes.find((n) => n.id === previewId) || selected}
      on:close={() => (sheet = null)}
      on:unpinned={() => push('ok', 'Pinned certificate cleared', 'the next connection measures it again')}
    />
  {:else if sheet === 'logs'}
    <LogSheet on:close={() => (sheet = null)} />
  {/if}

  <Toasts items={toasts} on:dismiss={dismissToast} />
</div>

<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { api, on, isMock } from './lib/bridge.js';
  import { icons } from './lib/icons.js';
  import { copyText } from './lib/utils.js';
  import TitleBar from './components/TitleBar.svelte';
  import StatusHero from './components/StatusHero.svelte';
  import NodeRow from './components/NodeRow.svelte';
  import ContextMenu from './components/ContextMenu.svelte';
  import ImportSheet from './components/ImportSheet.svelte';
  import SettingsSheet from './components/SettingsSheet.svelte';
  import ConfigSheet from './components/ConfigSheet.svelte';
  import LogSheet from './components/LogSheet.svelte';
  import StatusBar from './components/StatusBar.svelte';
  import Toasts from './components/Toasts.svelte';

  const dispatch = createEventDispatcher();

  let state = null;
  let nodes = [];
  let sheet = null;
  let menu = null;
  let previewId = '';
  let busy = false;
  let pingRunning = false;
  let lastPingAt = 0;
  let toasts = [];
  let elapsed = 0;
  let maximised = false;
  let tick;
  const offs = [];

  $: selected = nodes.find((n) => n.selected) || null;
  $: connected = !!state?.connected;

  onMount(async () => {
    try {
      state = await api.getState();
      nodes = (await api.listNodes()) || [];
    } catch (e) {
      push('err', 'Backend unavailable', String(e?.message || e));
    }
    offs.push(
      on('state', (s) => { state = s; busy = false; }),
      on('nodes', (list) => { nodes = list || []; }),
      on('ping', (r) => applyPing(r)),
      on('ping:done', () => { pingRunning = false; lastPingAt = Date.now(); }),
      on('log', (l) => { if (l?.level === 'error') push('err', 'Core', l.text); }),
    );
    tick = setInterval(() => {
      if (state?.connected && state?.startedAt) elapsed = Date.now() - state.startedAt;
      else elapsed = 0;
    }, 250);
    maximised = window.innerWidth >= 1600;
  });

  onDestroy(() => {
    clearInterval(tick);
    offs.forEach((f) => { try { f?.(); } catch (_) {} });
  });

  function applyPing(r) {
    if (!r) return;
    nodes = nodes.map((n) =>
      n.id === r.id ? { ...n, pingMs: r.ok ? r.ms : 0, pingError: r.error || '', pingAt: r.at || Date.now(), pinging: false } : n,
    );
  }

  function push(kind, msg, detail) {
    const id = Math.random().toString(36).slice(2);
    toasts = [...toasts, { id, kind, msg, detail }];
    setTimeout(() => dismissToast({ detail: id }), kind === 'err' ? 6500 : 3200);
  }
  function dismissToast(e) {
    toasts = toasts.filter((t) => t.id !== (e?.detail ?? e));
  }

  async function guard(fn, errPrefix) {
    try {
      return await fn();
    } catch (e) {
      push('err', errPrefix || 'Something went wrong', String(e?.message || e));
      return null;
    }
  }

  async function toggleConnect() {
    if (connected) {
      busy = true;
      try {
        const r = await guard(() => api.disconnect(), 'Disconnect');
        if (r !== null) { push('ok', 'Disconnected', 'The system proxy was restored'); }
      } finally {
        busy = false;
        state = (await guard(() => api.getState())) || state;
        nodes = (await api.listNodes()) || nodes;
      }
      return;
    }
    const targetNode = selected || nodes[0];
    if (!targetNode) { push('warn', 'Pick a node first', 'Click a row, then connect.'); return; }
    busy = true;
    try {
      const r = await guard(() => api.connect(targetNode.id), 'Connection failed');
      if (r) {
        state = r;
        push('ok', 'Connected', `${targetNode.name} · socks 127.0.0.1:${r.settings?.socksPort}` + (r.proxy?.enabledByApp ? ' · system proxy on' : ''));
      }
    } finally {
      busy = false;
      nodes = (await api.listNodes()) || nodes;
      state = (await guard(() => api.getState())) || state;
    }
  }

  async function connect(e) {
    const id = e?.detail || e;
    if (!id) return;
    busy = true;
    try {
      const r = await guard(() => api.connect(id), 'Connection failed');
      if (r) { state = r; }
    } finally {
      busy = false;
      nodes = (await api.listNodes()) || nodes;
      state = (await guard(() => api.getState())) || state;
    }
  }

  async function select(id) {
    nodes = nodes.map((n) => ({ ...n, selected: n.id === id }));
    await guard(() => api.selectNode(id), 'Select');
    state = (await guard(() => api.getState())) || state;
  }

  async function pingAll() {
    if (nodes.length === 0) return;
    pingRunning = true;
    nodes = nodes.map((n) => ({ ...n, pinging: true }));
    const r = await guard(() => api.pingAll(), 'Ping');
    if (r) nodes = (await api.listNodes()) || nodes;
    setTimeout(() => { pingRunning = false; lastPingAt = Date.now(); nodes = nodes.map((n) => ({ ...n, pinging: false })); }, 120);
  }

  async function pingOne(e) {
    const id = e?.detail || e;
    nodes = nodes.map((n) => (n.id === id ? { ...n, pinging: true } : n));
    const r = await guard(() => api.pingNode(id), 'Ping');
    if (r) applyPing({ ...r, pinging: false });
    nodes = nodes.map((n) => (n.id === r?.id ? { ...n, pinging: false } : n));
  }

  async function copyLink(e) {
    const id = e?.detail || e;
    const link = await guard(() => api.copyLink(id), 'Copy');
    if (!link) return;
    const ok = await copyText(link);
    push(ok ? 'ok' : 'err', ok ? 'Link copied' : 'Could not copy', ok ? '' : 'Clipboard unavailable');
  }

  async function remove(e) {
    const id = e?.detail || e;
    const node = nodes.find((n) => n.id === id);
    if (!node) return;
    if (node.connected && !window.confirm(`${node.name} is the active node.\nDisconnect and delete it?`)) return;
    await guard(() => api.deleteNode(id), 'Delete');
    nodes = (await api.listNodes()) || nodes.filter((n) => n.id !== id);
    push('ok', 'Deleted', node.name);
  }

  async function rename(e) {
    const node = nodes.find((n) => n.id === (e?.detail?.id ?? e?.detail) ) || selected;
    if (!node) return;
    const name = window.prompt('Rename node', node.name);
    if (name == null) return;
    const trimmed = name.trim();
    if (!trimmed) return;
    await guard(() => api.renameNode(node.id, trimmed), 'Rename');
    nodes = (await api.listNodes()) || nodes;
  }

  async function doImport(e) {
    const blob = e?.detail?.text ?? e?.detail ?? '';
    const res = await guard(() => api.importLinks(String(blob)), 'Import');
    if (!res) return;
    nodes = (await api.listNodes()) || nodes;
    state = (await guard(() => api.getState())) || state;
    if (res.added?.length) {
      push('ok', res.message || `Imported ${res.added.length} node(s)`, res.errors?.length ? `${res.errors.length} line(s) rejected` : '');
      sheet = null;
    } else {
      push('warn', res.message || 'Nothing imported', '');
    }
    return res;
  }

  async function saveSettings(e) {
    const r = await guard(() => api.updateSettings(e.detail), 'Settings');
    if (r) { state = r; push('ok', 'Settings saved'); sheet = null; }
  }

  function onMenuAction(e) {
    const { action, id } = e.detail || {};
    menu = null;
    switch (action) {
      case 'connect': connect(id); break;
      case 'disconnect': toggleConnect(); break;
      case 'ping': pingOne(id); break;
      case 'copy': copyLink(id); break;
      case 'rename': rename({ detail: id }); break;
      case 'preview': previewId = id; sheet = 'config'; break;
      case 'select': select(id); break;
      case 'delete': remove(id); break;
      default: break;
    }
  }

  function minimise() { api.win.minimise(); }
  function maximise() { maximised = !maximised; api.win.maximise(); }
  function onClose() { api.win.close(); }
  function onKey(ev) {
    if (ev.key === 'Escape') { menu = null; sheet = null; return; }
    if (ev.key === 'Enter' && (ev.ctrlKey || ev.metaKey) && sheet !== 'import') { toggleConnect(); return; }
    if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === 'i') { ev.preventDefault(); sheet = 'import'; }
    if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === 't') { ev.preventDefault(); pingAll(); }
  }
</script>
