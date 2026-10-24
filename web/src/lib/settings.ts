import type { Settings } from "../types";

const STORAGE_KEY = "niridesk.settings.v1";

export const defaultSettings: Settings = {
  quality: 7,
  compression: 2,
  fit: true,
  viewOnly: false,
};

function clamp(value: unknown, min: number, max: number, fallback: number): number {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.min(max, Math.max(min, Math.round(number)));
}

export function loadSettings(): Settings {
  try {
    const raw = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "{}") as Partial<Settings>;
    return {
      quality: clamp(raw.quality, 1, 9, defaultSettings.quality),
      compression: clamp(raw.compression, 0, 9, defaultSettings.compression),
      fit: typeof raw.fit === "boolean" ? raw.fit : defaultSettings.fit,
      viewOnly: typeof raw.viewOnly === "boolean" ? raw.viewOnly : defaultSettings.viewOnly,
    };
  } catch {
    return { ...defaultSettings };
  }
}

export function saveSettings(settings: Settings): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch {
    // Private browsing may reject storage; in-memory settings still work.
  }
}
