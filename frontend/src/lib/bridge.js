// bridge.js — the single place where the UI talks to the Go backend.
//
// Under Wails the runtime injects window.go.main.App.* (bound methods) and
// window.runtime.EventsOn (push events). Outside Wails — e.g. `npm run preview`,
// which is how this UI is built and checked in CI — the same surface is served by
// an in-memory mock so the frontend remains fully developable without a Windows
// toolchain. The mock is announced in the UI header so it can never be mistaken
// for a live connection.

const mock = import.meta.env.VITE_LIGHTVPN_MOCK === '1' || (typeof window !== 'undefined' && !window.go);

const listeners = new Map();
const state = {
  nodes: [
    {
      id: 'a1b2c3d4e5f6', name: 'DE · Frankfurt 01', protocol: 'vless', address: 'de1.example.net', port: 443,
      endpoint: 'de1.example.net:443', transport: 'WS · TLS', network: 'ws', security: 'tls',
      sni: 'de1.example.net', host: 'de1.example.net', path: '/v2rayws', fp: 'chrome', alpn: 'h2, http/1.1',
      pingMs: 14.2, pingError: '', pingAt: Date.now() - 4000, selected: true, connected: false, pinging: false,
    },
    {
      id: 'b2c3d4e5f6a1', name: 'NL · Amsterdam · Reality', protocol: 'vless', address: 'nl1.example.net', port: 8443,
      endpoint: 'nl1.example.net:8443', transport: 'TCP · REALITY', network: 'tcp', security: 'reality',
      sni: 'www.microsoft.com', flow: 'xtls-rprx-vision', pbk: 'AbC…xYz', sid: '0123abcd', spx: '/',
      pingMs: 32.6, pingError: '', pingAt: Date.now() - 4000, selected: false, connected: false, pinging: false,
    },
    {
      id: 'c3d4e5f6a1b2', name: 'FR · Paris · Trojan', protocol: 'trojan', address: 'fr1.example.net', port: 2053,
      endpoint: 'fr1.example.net:2053', transport: 'TCP · TLS', network: 'tcp', security: 'tls',
      sni: 'fr1.example.net', pingMs: 0, pingError: 'timed out (unreachable or blocked port)',
      allowInsecure: true, certPin: '9f2c1e77a0b34d5c8e9102a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7',
      certNote: 'pinned certificate fr1.example.net, valid until 2027-04-11', notes: ['flow=xtls-rprx-vision ignored: Xray no longer supports flow for Trojan'],
      pingAt: Date.now() - 4000, selected: false, connected: false, pinging: false,
    },
  ],
  connected: false,
  connecting: false,
  startedAt: 0,
  elapsedMs: 0,
  settings: {
    socksPort: 10808, httpPort: 10809, pingTimeoutMs: 3000, pingSamples: 3, systemProxy: true,
    proxyBypass: '127.0.0.1;localhost;192.168.*;<local>', bypassLocal: true, allowInsecureAll: false,
    sniffing: true, logLevel: 'warning', dnsServers: '223.5.5.5,1.1.1.1,8.8.8.8',
    minimizeToTray: true, connectOnStartup: false, autoPingOnStart: true,
  },
  proxy: { supported: true, enabledByApp: false, proxyEnable: false, proxyServer: '', proxyOverride: '', key: 'HKCU\\…\\Internet Settings' },
  logs: [],
};

let idSeq = 1;
const nid = () => 'mock' + (idSeq++).toString(16).padStart(8, '0');

function view(n) {
  return { ...n, connected: state.connected && n.selected, selected: !!n.selected };
}

function snapshot() {
  return {
    appName: 'LightVPN', appVersion: '1.0.0 (preview)', coreVersion: '26.3.27',
    connected: state.connected, connecting: state.connecting,
    elapsedMs: state.connected ? Date.now() - state.startedAt : 0,
    startedAt: state.startedAt,
    current: state.nodes.find((n) => n.selected) ? view(state.nodes.find((n) => n.selected)) : null,
    status: {
      running: state.connected, starting: state.connecting,
      nodeId: state.nodes.find((n) => n.selected)?.id || '',
      nodeName: state.nodes.find((n) => n.selected)?.name || '',
      socksPort: state.settings.socksPort, httpPort: state.settings.httpPort,
      startedAt: state.startedAt, uptimeMs: state.connected ? Date.now() - state.startedAt : 0,
      lastError: '',
    },
    proxy: state.proxy, settings: state.settings, pingRunning: false,
    platform: 'web/preview', storePath: '(in-memory preview)', warnings: [], error: '',
  };
}

function emit(event, data) {
  (listeners.get(event) || []).forEach((cb) => {
    try { cb(data); } catch (e) { console.warn('listener failed', e); }
  });
}

// Mock window/runtime so component code is identical in both modes.
if (mock && typeof window !== 'undefined') {
  const now = () => Date.now();
  window.go = {
    main: {
      App: {
        GetState: async () => snapshot(),
        ListNodes: async () => state.nodes.map(view),
        ImportLinks: async (blob) => {
          const lines = String(blob || '').split('\n').map((l) => l.trim()).filter(Boolean);
          const added = [];
          const errors = [];
          lines.forEach((l, i) => {
            const m = /^(vless|trojan):\/\/([^@]+)@([^:?]+):(\d+)([^#]*)#?(.*)$/.exec(l);
            if (!m) { errors.push({ line: i + 1, text: l.slice(0, 40), error: 'not a vless:// or trojan:// link' }); return; }
            const params = new URLSearchParams((m[5] || '').replace(/;/g, '&'));
            const net = params.get('type') || params.get('network') || 'tcp';
            const sec = params.get('security') || 'none';
            const node = {
              id: nid(), name: decodeURIComponent(m[6] || '') || `${m[3]}:${m[4]}`, protocol: m[1],
              address: m[3], port: +m[4], endpoint: `${m[3]}:${m[4]}`,
              transport: `${net.toUpperCase()} · ${sec.toUpperCase()}`, network: net, security: sec,
              sni: params.get('sni') || '', host: params.get('host') || '', path: params.get('path') || '',
              flow: params.get('flow') || '', fp: params.get('fp') || '', pbk: params.get('pbk') || '',
              sid: params.get('sid') || '', alpn: params.get('alpn') || '', pingMs: 0, pingError: '',
              pingAt: 0, selected: state.nodes.length === 0,
            };
            state.nodes.push(node);
            added.push(node);
          });
          emit('nodes', state.nodes.map(view));
          return { added: added.map(view), errors, total: lines.length, message: `Imported ${added.length} node${added.length === 1 ? '' : 's'}` };
        },
        DeleteNode: async (id) => {
          state.nodes = state.nodes.filter((n) => n.id !== id);
          if (!state.nodes.some((n) => n.selected) && state.nodes[0]) state.nodes[0].selected = true;
          emit('nodes', state.nodes.map(view));
          return null;
        },
        RenameNode: async (id, name) => {
          const n = state.nodes.find((x) => x.id === id);
          if (n) n.name = name;
          emit('nodes', state.nodes.map(view));
          return null;
        },
        CopyLink: async (id) => `vless://${(state.nodes.find((n) => n.id === id) || {}).address || 'example.net'}?mock=1`,
        SelectNode: async (id) => {
          state.nodes.forEach((n) => { n.selected = n.id === id; });
          emit('nodes', state.nodes.map(view));
          return null;
        },
        NodeConfigPreview: async (id) => JSON.stringify({ mock: true, id, note: 'the preview build does not run Xray' }, null, 2),
        ClearCertPin: async (id) => {
          const n = state.nodes.find((x) => x.id === id);
          if (n) { n.certPin = ''; n.certNote = ''; }
          return null;
        },
        PingNode: async (id) => {
          const n = state.nodes.find((x) => x.id === id) || state.nodes[0];
          const r = { id: n.id, name: n.name, endpoint: n.endpoint, ok: true, ms: +(8 + Math.random() * 90).toFixed(1), at: now() };
          n.pingMs = r.ms; n.pingError = ''; n.pingAt = r.at;
          emit('ping', r);
          return r;
        },
        PingAll: async () => {
          const out = [];
          for (const n of state.nodes) {
            await new Promise((r) => setTimeout(r, 120));
            const ok = Math.random() > 0.2;
            const r = { id: n.id, name: n.name, endpoint: n.endpoint, ok, ms: ok ? +(8 + Math.random() * 120).toFixed(1) : 0, error: ok ? '' : 'timed out (unreachable or blocked port)', at: now() };
            n.pingMs = r.ms; n.pingError = r.error; n.pingAt = r.at;
            out.push(r);
            emit('ping', r);
          }
          emit('nodes', state.nodes.map(view));
          return out;
        },
        CancelPing: async () => null,
        Connect: async () => {
          state.connected = true; state.startedAt = now();
          state.proxy = { ...state.proxy, enabledByApp: true, proxyEnable: true, proxyServer: `127.0.0.1:${state.settings.httpPort}` };
          emit('state', snapshot()); emit('nodes', state.nodes.map(view));
          return snapshot();
        },
        Disconnect: async () => {
          state.connected = false; state.startedAt = 0;
          state.proxy = { ...state.proxy, enabledByApp: false, proxyEnable: false, proxyServer: '' };
          emit('state', snapshot()); emit('nodes', state.nodes.map(view));
          return null;
        },
        Toggle: async () => (state.connected ? (await window.go.main.App.Disconnect(), snapshot()) : await window.go.main.App.Connect('')),
        SetSystemProxy: async (on) => { state.proxy = { ...state.proxy, proxyEnable: on }; return snapshot(); },
        ProxyStatus: async () => state.proxy,
        UpdateSettings: async (s) => { state.settings = { ...state.settings, ...s }; return snapshot(); },
        GetSettings: async () => state.settings,
        ConfigPath: async () => '(in-memory preview)',
        GetLogs: async (n) => state.logs.slice(-n),
        WindowMinimise: async () => null,
        WindowToggleMaximise: async () => null,
        WindowHide: async () => null,
        WindowShow: async () => null,
        WindowClose: async () => null,
        Quit: async () => null,
      },
    },
  };
  window.runtime = {
    EventsOn: (name, cb) => {
      if (!listeners.has(name)) listeners.set(name, []);
      listeners.get(name).push(cb);
      return () => listeners.set(name, (listeners.get(name) || []).filter((f) => f !== cb));
    },
    EventsOff: (name) => listeners.delete(name),
    EventsEmit: () => {},
  };
}

export const isMock = mock;

/** Call a bound Go method. */
export async function call(name, ...args) {
  const fn = window.go?.main?.App?.[name];
  if (!fn) throw new Error(`backend method ${name} is unavailable`);
  return fn(...args);
}

/** Subscribe to a backend event; returns an unsubscribe function. */
export function on(event, cb) {
  if (window.runtime?.EventsOn) return window.runtime.EventsOn(event, cb);
  if (!listeners.has(event)) listeners.set(event, []);
  listeners.get(event).push(cb);
  return () => listeners.set(event, (listeners.get(event) || []).filter((f) => f !== cb));
}

export const api = {
  getState: () => call('GetState'),
  listNodes: () => call('ListNodes'),
  importLinks: (blob) => call('ImportLinks', blob),
  deleteNode: (id) => call('DeleteNode', id),
  renameNode: (id, name) => call('RenameNode', id, name),
  copyLink: (id) => call('CopyLink', id),
  selectNode: (id) => call('SelectNode', id),
  preview: (id) => call('NodeConfigPreview', id),
  clearPin: (id) => call('ClearCertPin', id),
  pingNode: (id) => call('PingNode', id),
  pingAll: () => call('PingAll'),
  cancelPing: () => call('CancelPing'),
  connect: (id) => call('Connect', id || ''),
  disconnect: () => call('Disconnect'),
  setSystemProxy: (on_) => call('SetSystemProxy', on_),
  updateSettings: (s) => call('UpdateSettings', s),
  getLogs: (n) => call('GetLogs', n || 200),
  win: {
    minimise: () => call('WindowMinimise'),
    maximise: () => call('WindowToggleMaximise'),
    hide: () => call('WindowHide'),
    close: () => call('WindowClose'),
    quit: () => call('Quit'),
  },
};
