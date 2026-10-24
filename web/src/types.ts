export type ConnectionState = "idle" | "connecting" | "connected" | "error";

export type AuthState = "checking" | "online" | "offline";

export type SheetName = "settings" | "clipboard" | null;

export type ViewMode = "fit" | "fill" | "actual";

export type AudioState = "idle" | "starting" | "playing" | "error";

export interface Settings {
  quality: number;
  compression: number;
  viewMode: ViewMode;
  viewOnly: boolean;
  menuHidden: boolean;
  audioEnabled: boolean;
  audioDelay: number;
}

export interface OutputInfo {
  name: string;
  description: string;
  width: number;
  height: number;
  captured: boolean;
  power: string;
}

export interface WayvncState {
  managed: boolean;
  running: boolean;
  restarts: number;
  last_error?: string;
  version?: string;
  neatvnc?: string;
  aml?: string;
}

export interface VncStatus {
  reachable: boolean;
  wayvnc: WayvncState;
}

export interface WaylandStatus {
  display: string;
  socket_present: boolean;
}

export interface AudioStatus {
  enabled: boolean;
  available: boolean;
  active_clients: number;
  error?: string;
}

export interface PrivacyState {
  enabled: boolean;
  supported: boolean;
  last_error?: string;
}

export interface StatusResponse {
  version: string;
  commit: string;
  uptime_seconds: number;
  vnc: VncStatus;
  wayland: WaylandStatus;
  outputs: OutputInfo[];
  output_error?: string;
  active_clients: number;
  max_clients: number;
  max_fps: number;
  audio: AudioStatus;
  privacy: PrivacyState;
}

export interface OverlayState {
  kind: "connecting" | "error" | "idle";
  title: string;
  detail: string;
  retry: boolean;
}

export interface ToastMessage {
  id: number;
  message: string;
  type: "info" | "error";
}

export interface RfbOptions {
  shared?: boolean;
  credentials?: {
    username?: string;
    password?: string;
    target?: string;
  };
}

export interface RfbClient {
  background: string;
  scaleViewport: boolean;
  clipViewport: boolean;
  dragViewport: boolean;
  resizeSession: boolean;
  qualityLevel: number;
  compressionLevel: number;
  viewOnly: boolean;
  showDotCursor: boolean;
  focusOnClick: boolean;
  addEventListener(type: string, listener: (event: Event) => void): void;
  disconnect(): void;
  sendKey(keysym: number, code?: string, down?: boolean): void;
  clipboardPasteFrom(text: string): void;
  focus(options?: FocusOptions): void;
}

export interface RfbConstructor {
  new (target: HTMLElement, url: string, options?: RfbOptions): RfbClient;
}

export interface InstallPromptEvent extends Event {
  prompt(): Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed"; platform: string }>;
}
