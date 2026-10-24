import type { Settings, ViewMode } from "../types";

const STORAGE_KEY = "niridesk.settings.v1";

function prefersFilledViewport(): boolean {
  try {
    return window.matchMedia("(max-width: 960px), (pointer: coarse)").matches;
  } catch {
    return false;
  }
}

function isViewMode(value: unknown): value is ViewMode {
  return value === "fit" || value === "fill" || value === "actual";
}

export const defaultSettings: Settings = {
  quality: 7,
  compression: 2,
  viewMode: prefersFilledViewport() ? "fill" : "fit",
  viewOnly: false,
  menuHidden: false,
};

function clamp(value: unknown, min: number, max: number, fallback: number): number {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.min(max, Math.max(min, Math.round(number)));
}

export function loadSettings(): Settings {
  try {
    const raw = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "{}") as Partial<Settings> & {
      fit?: unknown;
    };
    const legacyFit = raw.fit;
    const viewMode = isViewMode(raw.viewMode)
      ? raw.viewMode
      : legacyFit === false
        ? "actual"
        : defaultSettings.viewMode;
    return {
      quality: clamp(raw.quality, 1, 9, defaultSettings.quality),
      compression: clamp(raw.compression, 0, 9, defaultSettings.compression),
      viewMode,
      viewOnly: typeof raw.viewOnly === "boolean" ? raw.viewOnly : defaultSettings.viewOnly,
      menuHidden: typeof raw.menuHidden === "boolean" ? raw.menuHidden : defaultSettings.menuHidden,
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
