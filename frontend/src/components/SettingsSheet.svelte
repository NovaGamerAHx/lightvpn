<svelte:window on:keydown={onKey} />

<div class="scrim" on:click|self={() => dispatch('close')} role="dialog" aria-modal="true" aria-label="Settings">
  <div class="sheet">
    <header>
      <h3>Settings</h3>
      <span class="sub">stored next to the EXE in <code>config.json</code></span>
      <span style="flex:1" />
      <button class="iconbtn" on:click={() => dispatch('close')} title="Close (Esc)" aria-label="Close">{@html icons.x}</button>
    </header>

    <div class="body">
      <div class="section-title">Inbound Ports & Network</div>
      <div class="grid2">
        <div class="field">
          <label for="socks">SOCKS5 inbound port</label>
          <input id="socks" type="number" min="1" max="65535" bind:value={s.socksPort} />
          <div class="note">127.0.0.1 only. Apps that speak SOCKS5 point here.</div>
        </div>
        <div class="field">
          <label for="http">HTTP proxy port</label>
          <input id="http" type="number" min="1" max="65535" bind:value={s.httpPort} />
          <div class="note">This is what the Windows system proxy is set to.</div>
        </div>
        <div class="field">
          <label for="ptimeout">Ping timeout (ms)</label>
          <input id="ptimeout" type="number" min="200" max="15000" step="100" bind:value={s.pingTimeoutMs} />
        </div>
        <div class="field">
          <label for="psamples">Ping samples (best of N)</label>
          <input id="psamples" type="number" min="1" max="10" bind:value={s.pingSamples} />
        </div>
        <div class="field" style="grid-column:1/-1">
          <label for="bypass">System proxy bypass list</label>
          <input id="bypass" type="text" bind:value={s.proxyBypass} spellcheck="false" />
          <div class="note">Written to <code>ProxyOverride</code> verbatim. <code>&lt;local&gt;</code> is the Windows wildcard for hostnames without a dot.</div>
        </div>
        <div class="field" style="grid-column:1/-1">
          <label for="dns">Xray DNS servers (comma separated)</label>
          <input id="dns" type="text" bind:value={s.dnsServers} spellcheck="false" />
          <div class="note">Used to resolve the server address of a node. Leave the defaults if unsure.</div>
        </div>
        <div class="field">
          <label for="loglevel">Core log level</label>
          <select id="loglevel" bind:value={s.logLevel}>
            {#each ['none', 'error', 'warning', 'info', 'debug'] as lvl}<option value={lvl}>{lvl}</option>{/each}
          </select>
          <div class="note">“info” shows handshake progress in the core log.</div>
        </div>
      </div>

      <div class="section-title" style="margin-top:16px">Routing & System Rules</div>
      <div style="display:grid;gap:9px">
        <button class="toggle" class:on={s.systemProxy} on:click={() => (s.systemProxy = !s.systemProxy)} type="button">
          <span class="switch" /><span class="txt"><b>Set the Windows system proxy</b>
            <span>HKCU Internet Settings → 127.0.0.1:{s.httpPort}, restored on disconnect. {proxyNote}</span></span>
        </button>
        <button class="toggle" class:on={s.bypassLocal} on:click={() => (s.bypassLocal = !s.bypassLocal)} type="button">
          <span class="switch" /><span class="txt"><b>Route LAN, loopback and private ranges direct</b>
            <span>Printers, NAS, localhost dev servers stay reachable. Turn off to force every byte through the tunnel.</span></span>
        </button>
        <button class="toggle" class:on={s.sniffing} on:click={() => (s.sniffing = !s.sniffing)} type="button">
          <span class="switch" /><span class="txt"><b>Sniff TLS/HTTP for routing</b>
            <span>Reads the SNI/Host of a connection so rules and the server see the real domain.</span></span>
        </button>
        <button class="toggle" class:on={s.allowInsecureAll} on:click={() => (s.allowInsecureAll = !s.allowInsecureAll)} type="button">
          <span class="switch" /><span class="txt"><b>Allow insecure TLS certificates</b>
            <span>Needed for self-signed servers. Off by default: it pins certificates instead of skipping verification.</span></span>
        </button>
      </div>

      <div class="section-title" style="margin-top:16px">Behavior & Automation</div>
      <div style="display:grid;gap:9px">
        <button class="toggle" class:on={s.minimizeToTray} on:click={() => (s.minimizeToTray = !s.minimizeToTray)} type="button">
          <span class="switch" /><span class="txt"><b>Close button hides to the tray</b>
            <span>The tunnel keeps running; use the tray menu to disconnect or exit.</span></span>
        </button>
        <button class="toggle" class:on={s.connectOnStartup} on:click={() => (s.connectOnStartup = !s.connectOnStartup)} type="button">
          <span class="switch" /><span class="txt"><b>Reconnect the last node on launch</b>
            <span>Starts the tunnel automatically when LightVPN opens.</span></span>
        </button>
        <button class="toggle" class:on={s.autoPingOnStart} on:click={() => (s.autoPingOnStart = !s.autoPingOnStart)} type="button">
          <span class="switch" /><span class="txt"><b>Test ping on launch</b>
            <span>Latency numbers are ready right after the list loads.</span></span>
        </button>
      </div>

      <div class="hint" style="margin-top:14px">
        Changing ports takes effect the next time you connect. config.json is the only file this app writes
        (besides the WebView2 profile) — deleting it resets everything.
      </div>
    </div>

    <footer>
      <span class="hint" style="margin:0">{storePath}</span>
      <span class="grow" />
      <button class="btn" on:click={() => dispatch('close')}>Cancel</button>
      <button class="btn primary" on:click={handleSave}>{@html icons.check} Save</button>
    </footer>
  </div>
</div>

<script>
  import { icons } from '../lib/icons.js';
  import { createEventDispatcher } from 'svelte';

  const defaults = {
    socksPort: 10808,
    httpPort: 10809,
    pingTimeoutMs: 3000,
    pingSamples: 3,
    systemProxy: true,
    proxyBypass: '127.0.0.1;localhost;192.168.*;<local>',
    bypassLocal: true,
    allowInsecureAll: false,
    sniffing: true,
    logLevel: 'warning',
    dnsServers: '1.1.1.1,8.8.8.8',
    minimizeToTray: true,
    connectOnStartup: false,
    autoPingOnStart: true,
  };

  export let settings = {};
  export let proxy = {};
  const dispatch = createEventDispatcher();

  let s = { ...defaults, ...(settings || {}) };
  $: storePath = s?.storePath || 'config.json';
  $: proxyNote = proxy?.supported === false
    ? 'Windows only.'
    : proxy?.enabledByApp
      ? `Active: ${proxy.proxyServer}`
      : 'Applied while connected.';

  function handleSave() {
    const socksPort = Math.max(1, Math.min(65535, parseInt(s.socksPort, 10) || 10808));
    const httpPort = Math.max(1, Math.min(65535, parseInt(s.httpPort, 10) || 10809));
    const pingTimeoutMs = Math.max(200, parseInt(s.pingTimeoutMs, 10) || 3000);
    const pingSamples = Math.max(1, Math.min(10, parseInt(s.pingSamples, 10) || 3));
    const payload = {
      ...s,
      socksPort,
      httpPort,
      pingTimeoutMs,
      pingSamples,
    };
    dispatch('save', payload);
  }

  function onKey(e) {
    if (e.key === 'Escape') dispatch('close');
  }
</script>
