export function pingClass(ms) {
  if (ms == null || ms <= 0) return 'bad';
  if (ms < 60) return 'good';
  if (ms < 150) return 'mid';
  return 'bad';
}

export function pingText(r) {
  if (!r) return '—';
  if (!r.ok) return 'timeout';
  return `${r.ms.toFixed(r.ms < 100 ? 1 : 0)} ms`;
}

export function fmtUptime(ms) {
  if (!ms || ms < 0) ms = 0;
  const s = Math.floor(ms / 1000);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const pad = (n) => String(n).padStart(2, '0');
  return h > 0 ? `${h}:${pad(m)}:${pad(sec)}` : `${pad(m)}:${pad(sec)}`;
}

export function fmtAgo(ts) {
  if (!ts) return 'never';
  const d = Math.max(0, Date.now() - ts);
  if (d < 45e3) return 'just now';
  if (d < 90 * 60e3) return `${Math.round(d / 60e3)} min ago`;
  if (d < 36 * 3600e3) return `${Math.round(d / 3600e3)} h ago`;
  return new Date(ts).toLocaleDateString();
}

export function protoBadge(protocol) {
  return protocol === 'trojan' ? 'Trojan' : 'VLESS';
}

export async function copyText(text) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (_) { /* fall through to the legacy path */ }
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand('copy');
    ta.remove();
    return ok;
  } catch (_) {
    return false;
  }
}

export function uid() {
  return Math.random().toString(36).slice(2, 10);
}
