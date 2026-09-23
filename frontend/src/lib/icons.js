const s = (d, extra = '') =>
  `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" ${extra}>${d}</svg>`;

export const icons = {
  shield: s('<path d="M12 3l7 3v6c0 4.2-2.9 7.7-7 9-4.1-1.3-7-4.8-7-9V6z"/><path d="M12 10.5v2.5"/>'),
  plus: s('<path d="M12 5v14M5 12h14"/>'),
  ping: s('<path d="M3 12h3l2.5-6 4 12L15 12h6"/>'),
  gear: s('<circle cx="12" cy="12" r="3.2"/><path d="M19.4 15a1.6 1.6 0 00.32 1.77l.06.06a2 2 0 11-2.83 2.83l-.06-.06a1.6 1.6 0 00-1.77-.32 1.6 1.6 0 00-1 1.47V22a2 2 0 11-4 0v-.1a1.6 1.6 0 00-1.05-1.47 1.6 1.6 0 00-1.77.32l-.06.06a2 2 0 11-2.83-2.83l.06-.06A1.6 1.6 0 004.6 15a1.6 1.6 0 00-1.47-1H3a2 2 0 110-4h.1A1.6 1.6 0 004.6 9a1.6 1.6 0 00-.32-1.77l-.06-.06a2 2 0 112.83-2.83l.06.06A1.6 1.6 0 009 4.6h.05A1.6 1.6 0 0010.5 3.1V3a2 2 0 114 0v.1a1.6 1.6 0 001 1.47 1.6 1.6 0 001.77-.32l.06-.06a2 2 0 112.83 2.83l-.06.06A1.6 1.6 0 0019.4 9v.05a1.6 1.6 0 001.47 1H21a2 2 0 110 4h-.1a1.6 1.6 0 00-1.5 1z"/>'),
  dots: s('<circle cx="12" cy="5.5" r="1.4" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="12" cy="18.5" r="1.4" fill="currentColor" stroke="none"/>'),
  copy: s('<rect x="9" y="9" width="11" height="11" rx="2.4"/><path d="M15 5.5A2.5 2.5 0 0012.5 3h-7A2.5 2.5 0 003 5.5v7A2.5 2.5 0 005.5 15"/>'),
  trash: s('<path d="M4 7h16M10 4h4M6 7l1 13h10l1-13M10 11v6M14 11v6"/>'),
  edit: s('<path d="M4 20h4l10.5-10.5a2.1 2.1 0 10-3-3L5 17z"/>'),
  eye: s('<path d="M2 12s3.6-6.5 10-6.5S22 12 22 12s-3.6 6.5-10 6.5S2 12 2 12z"/><circle cx="12" cy="12" r="2.6"/>'),
  play: s('<path d="M8 5.5l10 6.5-10 6.5z" fill="currentColor" stroke="none"/>'),
  stop: s('<rect x="6.5" y="6.5" width="11" height="11" rx="2.6" fill="currentColor" stroke="none"/>'),
  power: s('<path d="M12 3.5v8"/><path d="M17.6 6.9a7.5 7.5 0 11-11.2 0"/>'),
  x: s('<path d="M6 6l12 12M18 6L6 18"/>'),
  min: s('<path d="M5 12h14"/>'),
  max: s('<rect x="5.5" y="5.5" width="13" height="13" rx="2.4"/>'),
  restore: s('<rect x="4" y="7.5" width="10.5" height="10.5" rx="2"/><path d="M8 7.5V6a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2h-1.5"/>'),
  check: s('<path d="M4.5 12.5l5 5 10-11"/>'),
  warn: s('<path d="M12 3.8L21 20H3z"/><path d="M12 10v4.5M12 17.4v.2"/>'),
  info: s('<circle cx="12" cy="12" r="9"/><path d="M12 11v5.5M12 7.6v.2"/>'),
  link: s('<path d="M10 13.5a4 4 0 005.7 0l3-3a4 4 0 10-5.7-5.7L11.6 6.2"/><path d="M14 10.5a4 4 0 00-5.7 0l-3 3a4 4 0 105.7 5.7l1.3-1.4"/>'),
  terminal: s('<rect x="3" y="4.5" width="18" height="15" rx="2.6"/><path d="M7 10l3 2.5L7 15M12.5 15.5H17"/>'),
  refresh: s('<path d="M20 11a8 8 0 10-2.3 6"/><path d="M20 4.5V11h-6.5"/>'),
};
