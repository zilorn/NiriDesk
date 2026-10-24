import type { Settings, ViewMode } from "../types";

const STORAGE_KEY = "niridesk.settings.v1";
const VIEW_BOOTSTRAP_KEY = "niridesk.view-bootstrap.v1";
const CHROME_BOOTSTRAP_KEY = "niridesk.chrome-bootstrap.v1";

function consumeViewBootstrap(): boolean {
  try {
    if (localStorage.getItem(VIEW_BOOTSTRAP_KEY)) return false;
    localStorage.setItem(VIEW_BOOTSTRAP_KEY, "1");
    return true;
  } catch {
    return false;
  }
}

function consumeChromeBootstrap(): boolean {
  try {
    if (localStorage.getItem(CHROME_BOOTSTRAP_KEY)) return false;
    localStorage.setItem(CHROME_BOOTSTRAP_KEY, "1");
    return true;
  } catch {
    return false;
  }
}

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
  menuHidden: prefersFilledViewport(),
  audioEnabled: false,
  audioDelay: 120,
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
    const bootstrapView = consumeViewBootstrap();
    const bootstrapChrome = consumeChromeBootstrap();
    const viewMode = bootstrapView
      ? defaultSettings.viewMode
      : isViewMode(raw.viewMode)
        ? raw.viewMode
        : legacyFit === false
          ? "actual"
          : defaultSettings.viewMode;
    return {
      quality: clamp(raw.quality, 1, 9, defaultSettings.quality),
      compression: clamp(raw.compression, 0, 9, defaultSettings.compression),
      viewMode,
      viewOnly: typeof raw.viewOnly === "boolean" ? raw.viewOnly : defaultSettings.viewOnly,
      menuHidden: bootstrapChrome
        ? defaultSettings.menuHidden
        : typeof raw.menuHidden === "boolean"
          ? raw.menuHidden
          : defaultSettings.menuHidden,
      audioEnabled: typeof raw.audioEnabled === "boolean" ? raw.audioEnabled : defaultSettings.audioEnabled,
      audioDelay: clamp(raw.audioDelay, 40, 500, defaultSettings.audioDelay),
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
