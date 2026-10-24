import type { Component } from "solid-js";

export function IconSprite() {
  return (
    <svg class="svg-sprite" aria-hidden="true">
      <symbol id="i-monitor" viewBox="0 0 24 24"><rect x="2" y="3" width="20" height="14" rx="2" /><path d="M8 21h8M12 17v4" /></symbol>
      <symbol id="i-keyboard" viewBox="0 0 24 24"><rect x="2" y="6" width="20" height="12" rx="2" /><path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M6 14h.01M18 14h.01M9 14h6" /></symbol>
      <symbol id="i-maximize" viewBox="0 0 24 24"><path d="M8 3H5a2 2 0 0 0-2 2v3M16 3h3a2 2 0 0 1 2 2v3M8 21H5a2 2 0 0 1-2-2v-3M16 21h3a2 2 0 0 0 2-2v-3" /></symbol>
      <symbol id="i-minimize" viewBox="0 0 24 24"><path d="M8 3v3a2 2 0 0 1-2 2H3M16 3v3a2 2 0 0 0 2 2h3M8 21v-3a2 2 0 0 0-2-2H3M16 21v-3a2 2 0 0 1 2-2h3" /></symbol>
      <symbol id="i-fit" viewBox="0 0 24 24"><path d="M4 9V5a1 1 0 0 1 1-1h4M20 9V5a1 1 0 0 0-1-1h-4M4 15v4a1 1 0 0 0 1 1h4M20 15v4a1 1 0 0 1-1 1h-4" /><rect x="8" y="8" width="8" height="8" rx="1" /></symbol>
      <symbol id="i-clipboard" viewBox="0 0 24 24"><rect x="8" y="2" width="8" height="4" rx="1" /><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" /></symbol>
      <symbol id="i-sliders" viewBox="0 0 24 24"><path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6" /></symbol>
      <symbol id="i-logout" viewBox="0 0 24 24"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" /></symbol>
      <symbol id="i-refresh" viewBox="0 0 24 24"><path d="M21 2v6h-6M3 12a9 9 0 0 1 15-6.7L21 8M3 22v-6h6M21 12a9 9 0 0 1-15 6.7L3 16" /></symbol>
      <symbol id="i-x" viewBox="0 0 24 24"><path d="M18 6 6 18M6 6l12 12" /></symbol>
      <symbol id="i-check" viewBox="0 0 24 24"><path d="m20 6-11 11-5-5" /></symbol>
      <symbol id="i-copy" viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></symbol>
      <symbol id="i-send" viewBox="0 0 24 24"><path d="m22 2-7 20-4-9-9-4ZM22 2 11 13" /></symbol>
      <symbol id="i-lock" viewBox="0 0 24 24"><rect x="3" y="11" width="18" height="11" rx="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" /></symbol>
      <symbol id="i-shield" viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z" /><path d="m9 12 2 2 4-4" /></symbol>
      <symbol id="i-wifi" viewBox="0 0 24 24"><path d="M5 12.5a10 10 0 0 1 14 0M8.5 16a5 5 0 0 1 7 0M12 20h.01" /><path d="M2 9a15 15 0 0 1 20 0" /></symbol>
      <symbol id="i-mouse" viewBox="0 0 24 24"><rect x="5" y="2" width="14" height="20" rx="7" /><path d="M12 6v4" /></symbol>
      <symbol id="i-phone" viewBox="0 0 24 24"><rect x="5" y="2" width="14" height="20" rx="3" /><path d="M12 18h.01" /></symbol>
      <symbol id="i-command" viewBox="0 0 24 24"><path d="M9 6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3Z" /></symbol>
      <symbol id="i-eye" viewBox="0 0 24 24"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></symbol>
      <symbol id="i-eye-off" viewBox="0 0 24 24"><path d="m3 3 18 18M10.6 10.6a2 2 0 0 0 2.8 2.8M9.9 5.2A9.7 9.7 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3 3.9M6.6 6.6A18 18 0 0 0 2 12s3.5 7 10 7a10 10 0 0 0 4.5-1.1" /></symbol>
      <symbol id="i-volume" viewBox="0 0 24 24"><path d="M11 5 6 9H3v6h3l5 4V5Z" /><path d="M15.5 8.5a5 5 0 0 1 0 7M18.5 5.5a9 9 0 0 1 0 13" /></symbol>
      <symbol id="i-volume-off" viewBox="0 0 24 24"><path d="M11 5 6 9H3v6h3l5 4V5Z" /><path d="m16 9 5 5M21 9l-5 5" /></symbol>
      <symbol id="i-download" viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3" /></symbol>
      <symbol id="i-chevron" viewBox="0 0 24 24"><path d="m9 18 6-6-6-6" /></symbol>
      <symbol id="i-chevron-down" viewBox="0 0 24 24"><path d="m6 9 6 6 6-6" /></symbol>
      <symbol id="i-layers" viewBox="0 0 24 24"><path d="m12 2 9 5-9 5-9-5Z" /><path d="m3 12 9 5 9-5M3 17l9 5 9-5" /></symbol>
      <symbol id="i-menu" viewBox="0 0 24 24"><path d="M4 6h16M4 12h16M4 18h16" /></symbol>
    </svg>
  );
}

export const Icon: Component<{ name: string; class?: string }> = (props) => (
  <svg class={`icon${props.class ? ` ${props.class}` : ""}`} aria-hidden="true">
    <use href={`#i-${props.name}`} />
  </svg>
);

export function BrandMark(props: { large?: boolean }) {
  return (
    <span class={`brand-mark${props.large ? " brand-mark-large" : ""}`} aria-hidden="true">
      <svg viewBox="0 0 40 40" role="img">
        <path
          d="M11 12v16M11 20l10-8v16M29 12v16"
          fill="none"
          stroke="currentColor"
          stroke-width="3"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
        <circle cx="29" cy="30" r="2.4" fill="currentColor" />
      </svg>
    </span>
  );
}
