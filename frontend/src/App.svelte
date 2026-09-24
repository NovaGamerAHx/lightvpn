<svelte:window on:keydown={onKey} on:resize={onWindowResize} />

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

    {#if nodes.length > 1}
      <div class="search-box">
        <input
          class="search-input"
          type="text"
          placeholder="Filter nodes…"
          bind:value={search}
          spellcheck="false"
        />
        {#if search}
          <button class="clear-search" on:click={() => (search = '')} title="Clear filter">×</button>
        {/if}
      </div>
    {/if}

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
        <p>Paste one or many <code>vless://</code> or <code>trojan://</code> share links or a Base64 subscription list. Traffic flows only through the tunnel you start.</p>
        <button class="btn primary" on:click={() => (sheet = 'import')}>{@html icons.plus} Import links</button>
      </div>
    {:else if filteredNodes.length === 0}
      <div class="empty">
        {@html icons.eye}
        <h2>No matching nodes</h2>
        <p>No nodes match “{search}”. Clear the filter to show all {nodes.length} nodes.</p>
        <button class="btn" on:click={() => (search = '')}>Clear filter</button>
      </div>
    {:else}
      <div class="group">
        Nodes ({filteredNodes.length}) · click to select, double-click to connect, right-click for options
      </div>
      {#each filteredNodes as n (n.id)}
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
          on:menu={(e) => onRowMenu(e.detail)}
        />
      {/each}
    {/if}
  </div>

  <StatusBar
    {state}
    {elapsed}
    on:logs={() => (sheet = 'logs')}
    on:config={() => { previewId = selected?.id || ''; sheet = 'config'; }}
  />

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
  import { onMount, onDestroy } from 'svelte';
  import { api, on } from './lib/bridge.js';
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
  let search = '';
  let tick;
  const offs = [];

  $: selected = nodes.find((n) => n.selected) || (nodes.length > 0 ? nodes[0] : null);
  $: connected = !!state?.connected;

  $: filteredNodes = search.trim()
    ? nodes.filter((n) => {
        const q = search.toLowerCase();
        return (
          (n.name || '').toLowerCase().includes(q) ||
          (n.endpoint || '').toLowerCase().includes(q) ||
          (n.address || '').toLowerCase().includes(q) ||
          (n.protocol || '').toLowerCase().includes(q) ||
          (n.transport || '').toLowerCase().includes(q)
        );
      })
    : nodes;

  async function syncState() {
    try {
      const [s, n] = await Promise.all([
        api.getState(),
        api.listNodes(),
      ]);
      if (s) state = s;
      if (n) nodes = n;
    } catch (_) {}
  }

  onMount(async () => {
    await syncState();

    offs.push(
      on('state', (s) => {
        if (s) {
          state = s;
          busy = false;
        }
      }),
      on('nodes', (list) => {
        if (list) nodes = list;
      }),
      on('ping', (r) => applyPing(r)),
      on('ping:done', () => {
        pingRunning = false;
        lastPingAt = Date.now();
      }),
      on('log', (l) => {
        if (l?.level === 'error') push('err', 'Core Error', l.text);
      }),
    );

    // Sync state whenever the window regains focus
    const onFocus = () => {
      if (!busy) syncState();
    };
    window.addEventListener('focus', onFocus);
    document.addEventListener('visibilitychange', onFocus);
    offs.push(() => {
      window.removeEventListener('focus', onFocus);
      document.removeEventListener('visibilitychange', onFocus);
    });

    // Uptime tick & periodic background sync every ~2 seconds
    let pollCount = 0;
    tick = setInterval(() => {
      if (state?.connected && state?.startedAt) {
        elapsed = Date.now() - state.startedAt;
      } else if (elapsed !== 0) {
        elapsed = 0;
      }

      pollCount++;
      if (pollCount % 8 === 0 && !busy && !pingRunning) {
        syncState();
      }
    }, 250);

    onWindowResize();
  });

  onDestroy(() => {
    clearInterval(tick);
    offs.forEach((f) => {
      try { f?.(); } catch (_) {}
    });
  });

  function onWindowResize() {
    maximised = window.innerWidth >= (screen?.availWidth || 1920) - 20 &&
                window.innerHeight >= (screen?.availHeight || 1080) - 20;
  }

  function applyPing(r) {
    if (!r) return;
    nodes = nodes.map((n) =>
      n.id === r.id
        ? { ...n, pingMs: r.ok ? r.ms : 0, pingError: r.error || '', pingAt: r.at || Date.now(), pinging: false }
        : n,
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
    if (busy) return;
    const targetNode = selected || nodes[0];
    const targetId = targetNode?.id || '';
    if (!connected && !targetId) {
      push('warn', 'Pick a node first', 'Import or select a node, then connect.');
      return;
    }

    busy = true;
    try {
      const r = await guard(() => api.toggle(targetId), connected ? 'Disconnect failed' : 'Connection failed');
      if (r) {
        state = r;
        if (r.connected) {
          const activeNode = nodes.find((n) => n.id === (r.current?.id || targetId)) || targetNode;
          push(
            'ok',
            'Connected',
            `${activeNode?.name || 'Node'} · socks 127.0.0.1:${r.settings?.socksPort}` +
              (r.proxy?.enabledByApp ? ' · system proxy on' : ''),
          );
        } else {
          push('ok', 'Disconnected', 'The system proxy was restored');
        }
      }
    } finally {
      busy = false;
      await syncState();
    }
  }

  async function connect(e) {
    const id = e?.detail || e;
    if (!id || busy) return;
    busy = true;
    try {
      const r = await guard(() => api.connect(id), 'Connection failed');
      if (r) {
        state = r;
        if (r.connected) {
          const target = nodes.find((n) => n.id === id);
          push(
            'ok',
            'Connected',
            `${target?.name || 'Node'} · socks 127.0.0.1:${r.settings?.socksPort}` +
              (r.proxy?.enabledByApp ? ' · system proxy on' : ''),
          );
        }
      }
    } finally {
      busy = false;
      await syncState();
    }
  }

  async function select(id) {
    if (!id) return;
    nodes = nodes.map((n) => ({ ...n, selected: n.id === id }));
    await guard(() => api.selectNode(id), 'Select');
    state = (await guard(() => api.getState())) || state;
  }

  async function pingAll() {
    if (nodes.length === 0 || pingRunning) return;
    pingRunning = true;
    nodes = nodes.map((n) => ({ ...n, pinging: true }));
    try {
      const r = await guard(() => api.pingAll(), 'Ping');
      if (r) nodes = (await api.listNodes()) || nodes;
    } finally {
      pingRunning = false;
      lastPingAt = Date.now();
      nodes = nodes.map((n) => ({ ...n, pinging: false }));
    }
  }

  async function pingOne(e) {
    const id = e?.detail || e;
    if (!id) return;
    nodes = nodes.map((n) => (n.id === id ? { ...n, pinging: true } : n));
    try {
      const r = await guard(() => api.pingNode(id), 'Ping');
      if (r) applyPing({ ...r, pinging: false });
    } finally {
      nodes = nodes.map((n) => (n.id === id ? { ...n, pinging: false } : n));
    }
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
    push('ok', 'Deleted', node.name);
    await syncState();
  }

  async function rename(e) {
    const node = nodes.find((n) => n.id === (e?.detail?.id ?? e?.detail)) || selected;
    if (!node) return;
    const name = window.prompt('Rename node', node.name);
    if (name == null) return;
    const trimmed = name.trim();
    if (!trimmed) return;
    await guard(() => api.renameNode(node.id, trimmed), 'Rename');
    await syncState();
  }

  async function doImport(e) {
    const blob = e?.detail?.text ?? e?.detail ?? '';
    const res = await guard(() => api.importLinks(String(blob)), 'Import');
    if (!res) return;
    await syncState();
    if (res.added?.length) {
      push(
        'ok',
        res.message || `Imported ${res.added.length} node(s)`,
        res.errors?.length ? `${res.errors.length} line(s) rejected` : '',
      );
      sheet = null;
    } else {
      push('warn', res.message || 'Nothing imported', res.errors?.[0]?.error || '');
    }
    return res;
  }

  async function saveSettings(e) {
    const r = await guard(() => api.updateSettings(e.detail), 'Settings');
    if (r) {
      state = r;
      push('ok', 'Settings saved');
      sheet = null;
      await syncState();
    }
  }

  function onRowMenu(detail) {
    if (menu && menu.id === detail.id && detail.isButton) {
      menu = null;
    } else {
      menu = detail;
    }
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
    if (ev.key === 'Escape') {
      if (menu) { menu = null; return; }
      if (sheet) { sheet = null; return; }
      if (search) { search = ''; return; }
    }
    if (ev.key === 'Enter' && (ev.ctrlKey || ev.metaKey) && sheet !== 'import') {
      ev.preventDefault();
      toggleConnect();
      return;
    }
    if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === 'i' && !sheet) {
      ev.preventDefault();
      sheet = 'import';
      return;
    }
    if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === 't' && !sheet) {
      ev.preventDefault();
      pingAll();
      return;
    }
    if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === 'f' && !sheet) {
      ev.preventDefault();
      const el = document.querySelector('.search-input');
      if (el) el.focus();
    }
  }
</script>
