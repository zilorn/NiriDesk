import { createSignal, onCleanup, onMount } from "solid-js";
import { createStore } from "solid-js/store";
import { api, ApiError, errorMessage, isUnauthorized } from "./lib/api";
import { installFillViewport, remoteFramebufferSize } from "./lib/fillViewport";
import type { FillViewport } from "./lib/fillViewport";
import { MODIFIER_KEYS, SPECIAL_KEYS, keysymForCharacter } from "./lib/keys";
import type { ModifierName, SpecialName } from "./lib/keys";
import { loadSettings, saveSettings } from "./lib/settings";
import type {
  AuthState,
  ConnectionState,
  InstallPromptEvent,
  OutputInfo,
  OverlayState,
  RfbClient,
  RfbConstructor,
  Settings,
  SheetName,
  StatusResponse,
  ToastMessage,
  ViewMode,
} from "./types";

interface WakeLockLike {
  release(): Promise<void>;
  addEventListener(type: "release", listener: () => void): void;
}

const WAKE_LOCK = "wakeLock" in navigator;

const VIEW_MODE_LABELS: Record<ViewMode, string> = {
  fit: "适应窗口",
  fill: "铺满屏幕",
  actual: "1:1 拖动",
};

export function createAppController() {
  const [authenticated, setAuthenticated] = createSignal<boolean | null>(null);
  const [loginError, setLoginError] = createSignal("");
  const [loginPending, setLoginPending] = createSignal(false);
  const [authState, setAuthState] = createSignal<AuthState>("checking");
  const [version, setVersion] = createSignal("dev");
  const [connection, setConnection] = createSignal<ConnectionState>("idle");
  const [overlay, setOverlay] = createSignal<OverlayState | null>(null);
  const [resolution, setResolution] = createSignal("—");
  const [desktopName, setDesktopName] = createSignal("niri remote");
  const [status, setStatus] = createSignal<StatusResponse | null>(null);
  const [sheet, setSheet] = createSignal<SheetName>(null);
  const [settings, setSettings] = createStore<Settings>(loadSettings());
  const [modifiers, setModifiers] = createSignal<ModifierName[]>([]);
  const [beltOpen, setBeltOpen] = createSignal(false);
  const [keyboardActive, setKeyboardActiveSignal] = createSignal(false);
  const [clipboard, setClipboard] = createSignal("");
  const [toasts, setToasts] = createSignal<ToastMessage[]>([]);
  const [fullscreen, setFullscreen] = createSignal(false);
  const [installPrompt, setInstallPrompt] = createSignal<InstallPromptEvent | null>(null);
  const [switchingOutput, setSwitchingOutput] = createSignal<string | null>(null);

  let screenEl: HTMLElement | null = null;
  let keyboardEl: HTMLInputElement | null = null;
  let rfb: RfbClient | null = null;
  let fillViewport: FillViewport | null = null;
  let generation = 0;
  let statusTimer: number | undefined;
  let wakeLock: WakeLockLike | null = null;
  let toastId = 0;
  const toastTimers = new Map<number, number>();

  const outputs = (): OutputInfo[] => status()?.outputs ?? [];
  const currentOutput = (): OutputInfo | null => outputs().find((output) => output.captured) ?? null;
  const buildLabel = () => {
    const current = status();
    const build = current?.version || version();
    const commit = current?.commit;
    return commit && commit !== "unknown" ? `${build} · ${commit.slice(0, 7)}` : build;
  };
  const authStatusLabel = () => {
    switch (authState()) {
      case "online": return "桌面服务在线";
      case "offline": return "无法联系桌面服务";
      default: return "正在检查桌面服务";
    }
  };
  const connectionLabel = () => {
    switch (connection()) {
      case "connected": return "已连接";
      case "connecting": return "正在连接";
      case "error": return "连接中断";
      default: return "未连接";
    }
  };

  onMount(() => {
    void checkHealth();
    void restoreSession();
    registerServiceWorker();

    const fullscreenHandler = () => {
      const active = Boolean(document.fullscreenElement);
      setFullscreen(active);
      if (active) setMenuHidden(true);
    };
    const visibilityHandler = () => {
      if (document.hidden) {
        clearModifiers();
        void releaseWakeLock();
      } else if (connection() === "connected") {
        void requestWakeLock();
      }
    };
    const installHandler = (event: Event) => {
      event.preventDefault();
      setInstallPrompt(event as InstallPromptEvent);
    };

    document.addEventListener("fullscreenchange", fullscreenHandler);
    document.addEventListener("visibilitychange", visibilityHandler);
    window.addEventListener("beforeinstallprompt", installHandler);

    onCleanup(() => {
      document.removeEventListener("fullscreenchange", fullscreenHandler);
      document.removeEventListener("visibilitychange", visibilityHandler);
      window.removeEventListener("beforeinstallprompt", installHandler);
      stopStatusPolling();
      teardownRemote();
      for (const timer of toastTimers.values()) window.clearTimeout(timer);
      toastTimers.clear();
    });
  });

  async function checkHealth() {
    setAuthState("checking");
    try {
      const health = await api<{ status: string; version?: string }>("/api/health");
      setAuthState("online");
      if (health.version) setVersion(health.version);
    } catch {
      setAuthState("offline");
    }
  }

  async function restoreSession() {
    try {
      const session = await api<{ authenticated: boolean }>("/api/session");
      setAuthenticated(session.authenticated);
    } catch (error) {
      setAuthenticated(false);
      setLoginError(errorMessage(error));
    }
  }

  async function login(password: string) {
    if (loginPending()) return;
    setLoginPending(true);
    setLoginError("");
    try {
      await api("/api/login", { method: "POST", body: { password } });
      setAuthenticated(true);
      setAuthState("online");
    } catch (error) {
      setLoginError(errorMessage(error));
    } finally {
      setLoginPending(false);
    }
  }

  async function logout() {
    stopStatusPolling();
    teardownRemote();
    setAuthenticated(false);
    setLoginError("已退出登录。");
    setConnection("idle");
    setStatus(null);
    setSheet(null);
    setClipboard("");
    document.title = "NiriDesk · niri 远程桌面";
    try {
      await api("/api/logout", { method: "POST" });
    } catch {
      // The browser cookie is cleared locally even if the network request fails.
    }
    void checkHealth();
  }

  function expireSession() {
    if (authenticated() !== true) return;
    stopStatusPolling();
    teardownRemote();
    setAuthenticated(false);
    setLoginError("会话已过期，请重新登录。");
    setConnection("idle");
    setStatus(null);
    setSheet(null);
    document.title = "NiriDesk · niri 远程桌面";
  }

  function attachScreen(element: HTMLElement | null) {
    screenEl = element;
  }

  function attachKeyboard(element: HTMLInputElement | null) {
    keyboardEl = element;
  }

  function startRemote() {
    startStatusPolling();
    void connect();
  }

  function teardownRemote() {
    stopStatusPolling();
    disconnectSession(false);
    screenEl = null;
    keyboardEl = null;
    setSheet(null);
    setOverlay(null);
    clearModifiers();
    void releaseWakeLock();
  }

  async function refreshStatus() {
    if (authenticated() !== true) return;
    try {
      const next = await api<StatusResponse>("/api/status");
      setStatus(next);
      if (next.version) setVersion(next.version);
      window.setTimeout(updateResolution, 20);
    } catch (error) {
      if (isUnauthorized(error)) expireSession();
    }
  }

  function startStatusPolling() {
    stopStatusPolling();
    void refreshStatus();
    statusTimer = window.setInterval(() => void refreshStatus(), 20000);
  }

  function stopStatusPolling() {
    if (statusTimer !== undefined) {
      window.clearInterval(statusTimer);
      statusTimer = undefined;
    }
  }

  async function connect() {
    if (connection() === "connecting" || connection() === "connected" || rfb) return;
    if (!screenEl) {
      setConnection("error");
      setOverlay({
        kind: "error",
        title: "无法初始化桌面画布",
        detail: "请刷新页面后重试。",
        retry: true,
      });
      return;
    }

    const token = ++generation;
    setConnection("connecting");
    setOverlay({
      kind: "connecting",
      title: "正在连接桌面",
      detail: "建立加密控制通道…",
      retry: false,
    });

    let RFB: RfbConstructor;
    try {
      const module = await import("@novnc/novnc");
      RFB = module.default;
    } catch {
      if (token !== generation) return;
      setConnection("error");
      setOverlay({
        kind: "error",
        title: "无法载入远程桌面组件",
        detail: "前端资源不完整，请重新构建或刷新页面。",
        retry: true,
      });
      return;
    }

    if (token !== generation || !screenEl) return;

    let instance: RfbClient;
    try {
      const scheme = window.location.protocol === "https:" ? "wss" : "ws";
      instance = new RFB(screenEl, `${scheme}://${window.location.host}/websocket`, {
        shared: true,
        credentials: { password: "" },
      });
    } catch {
      setConnection("error");
      setOverlay({
        kind: "error",
        title: "无法创建远程会话",
        detail: "浏览器不支持所需的 WebSocket 或 Canvas 功能。",
        retry: true,
      });
      return;
    }

    rfb = instance;
    fillViewport = installFillViewport(instance);
    instance.background = "radial-gradient(circle at 50% 44%, #0b1611 0%, #040706 70%)";
    instance.showDotCursor = true;
    instance.focusOnClick = true;
    instance.qualityLevel = settings.quality;
    instance.compressionLevel = settings.compression;
    instance.viewOnly = settings.viewOnly;
    applyViewMode();

    instance.addEventListener("connect", () => {
      if (rfb !== instance) return;
      setConnection("connected");
      setOverlay(null);
      setResolutionText();
      void refreshStatus();
      void requestWakeLock();
      if (!window.matchMedia("(pointer: coarse)").matches && !settings.viewOnly) instance.focus();
      toast("已连接到 niri 桌面");
    });

    instance.addEventListener("disconnect", (event) => {
      if (rfb !== instance) return;
      rfb = null;
      fillViewport = null;
      clearModifiers();
      void releaseWakeLock();
      setConnection("error");
      setResolution("—");
      const clean = Boolean((event as CustomEvent<{ clean?: boolean }>).detail?.clean);
      setOverlay({
        kind: "error",
        title: "与桌面的连接已断开",
        detail: clean ? "远端关闭了会话。" : "网络或桌面服务中断，请检查服务状态后重试。",
        retry: true,
      });
    });

    instance.addEventListener("securityfailure", (event) => {
      if (rfb !== instance) return;
      const detail = (event as CustomEvent<{ reason?: string }>).detail;
      setConnection("error");
      setOverlay({
        kind: "error",
        title: "认证失败",
        detail: detail?.reason || "无法完成安全握手。",
        retry: true,
      });
    });

    instance.addEventListener("credentialsrequired", () => {
      if (rfb !== instance) return;
      setConnection("error");
      setOverlay({
        kind: "error",
        title: "需要额外凭据",
        detail: "wayvnc 要求额外认证，请检查部署配置。",
        retry: true,
      });
    });

    instance.addEventListener("desktopname", (event) => {
      const name = (event as CustomEvent<{ name?: string }>).detail?.name || "niri remote";
      setDesktopName(name);
      document.title = `${name} · NiriDesk`;
    });

    instance.addEventListener("clipboard", (event) => {
      const text = String((event as CustomEvent<{ text?: string }>).detail?.text ?? "");
      setClipboard(text);
      if (sheet() === "clipboard") toast("已收到远端剪贴板");
    });

    instance.addEventListener("bell", () => toast("远端发出提示音"));
  }

  function disconnectSession(showNotice: boolean) {
    const instance = rfb;
    generation++;
    rfb = null;
    fillViewport = null;
    if (instance) {
      try {
        instance.disconnect();
      } catch {
        // The socket may already be closed.
      }
    }
    clearModifiers();
    void releaseWakeLock();
    setResolution("—");
    setConnection("idle");
    setOverlay(
      showNotice
        ? { kind: "idle", title: "连接已关闭", detail: "可以随时重新连接。", retry: true }
        : null,
    );
  }

  function reconnect() {
    closeSheets();
    disconnectSession(false);
    setOverlay({
      kind: "connecting",
      title: "正在连接桌面",
      detail: "建立加密控制通道…",
      retry: false,
    });
    setConnection("connecting");
    window.setTimeout(() => void connect(), 0);
  }

  function applyViewMode() {
    if (!rfb) return;
    const instance = rfb;
    fillViewport?.setActive(false);

    if (settings.viewMode === "fill" && fillViewport?.supported) {
      instance.scaleViewport = false;
      instance.clipViewport = true;
      instance.dragViewport = true;
      fillViewport.setActive(true);
    } else if (settings.viewMode === "actual") {
      instance.scaleViewport = false;
      instance.clipViewport = true;
      instance.dragViewport = true;
    } else {
      instance.scaleViewport = true;
      instance.clipViewport = false;
      instance.dragViewport = false;
    }
    window.setTimeout(updateResolution, 60);
  }

  function setResolutionText() {
    const framebuffer = remoteFramebufferSize(rfb);
    if (framebuffer) {
      setResolution(`${framebuffer.width} × ${framebuffer.height}`);
      return;
    }
    const canvas = screenEl?.querySelector("canvas");
    setResolution(canvas && canvas.width && canvas.height ? `${canvas.width} × ${canvas.height}` : "—");
  }

  function updateResolution() {
    setResolutionText();
  }

  function updateSetting<K extends keyof Settings>(key: K, value: Settings[K]) {
    setSettings({ [key]: value } as Pick<Settings, K>);
    saveSettings(settings);
  }

  function setQuality(value: number) {
    const quality = Math.min(9, Math.max(1, Math.round(value)));
    updateSetting("quality", quality);
    if (rfb) rfb.qualityLevel = quality;
  }

  function setCompression(value: number) {
    const compression = Math.min(9, Math.max(0, Math.round(value)));
    updateSetting("compression", compression);
    if (rfb) rfb.compressionLevel = compression;
  }

  function viewModeLabel(mode: ViewMode = settings.viewMode): string {
    return VIEW_MODE_LABELS[mode];
  }

  function setViewMode(mode: ViewMode) {
    if (settings.viewMode === mode) return;
    updateSetting("viewMode", mode);
    applyViewMode();
    toast(`画面缩放：${viewModeLabel(mode)}`);
  }

  function cycleViewMode() {
    const order: ViewMode[] = ["fit", "fill", "actual"];
    const index = order.indexOf(settings.viewMode);
    setViewMode(order[(index + 1) % order.length]);
  }

  function setMenuHidden(hidden: boolean) {
    if (settings.menuHidden === hidden) return;
    updateSetting("menuHidden", hidden);
    if (hidden) {
      setModifierBeltOpen(false);
      clearModifiers();
      keyboardEl?.blur();
    }
  }

  function hideMenu() {
    setMenuHidden(true);
  }

  function showMenu() {
    setMenuHidden(false);
  }

  function toggleMenu() {
    setMenuHidden(!settings.menuHidden);
  }

  function setViewOnly(value: boolean) {
    updateSetting("viewOnly", value);
    if (rfb) rfb.viewOnly = value;
    if (value) {
      clearModifiers();
      keyboardEl?.blur();
      toast("已开启只看不控");
    } else {
      toast("已恢复远程控制");
    }
  }

  function isModifierActive(name: ModifierName): boolean {
    return modifiers().includes(name);
  }

  function toggleModifier(name: ModifierName) {
    const key = MODIFIER_KEYS[name];
    if (!rfb || connection() !== "connected" || settings.viewOnly) {
      toast("尚未连接桌面", "error");
      return;
    }
    const active = modifiers().includes(name);
    try {
      rfb.sendKey(key.keysym, key.code, !active);
    } catch {
      return;
    }
    setModifiers(active ? modifiers().filter((item) => item !== name) : [...modifiers(), name]);
  }

  function clearModifiers() {
    const instance = rfb;
    if (instance && connection() === "connected" && !settings.viewOnly) {
      for (const name of modifiers()) {
        const key = MODIFIER_KEYS[name];
        try {
          instance.sendKey(key.keysym, key.code, false);
        } catch {
          // Best effort while the transport is closing.
        }
      }
    }
    setModifiers([]);
  }

  function sendSpecial(name: SpecialName, showError = true) {
    const key = SPECIAL_KEYS[name];
    if (!rfb || connection() !== "connected" || settings.viewOnly) {
      if (showError) toast("尚未连接桌面", "error");
      return;
    }
    rfb.sendKey(key.keysym, key.code);
  }

  function sendText(text: string) {
    if (!rfb || connection() !== "connected" || settings.viewOnly) return;
    for (const character of Array.from(text)) {
      if (character === "\n") {
        sendSpecial("enter", false);
        continue;
      }
      if (character === "\t") {
        sendSpecial("tab", false);
        continue;
      }
      if (character === "\r") continue;
      rfb.sendKey(keysymForCharacter(character), "Unidentified");
    }
  }

  function setKeyboardActive(active: boolean) {
    setKeyboardActiveSignal(active);
    if (active) setModifierBeltOpen(true);
  }

  function toggleKeyboard() {
    if (settings.viewOnly) {
      toast("只看不控模式下不能输入", "error");
      return;
    }
    if (!window.matchMedia("(pointer: coarse)").matches) {
      if (rfb && connection() === "connected") {
        rfb.focus();
        toast("远程键盘已就绪，可直接输入");
      } else {
        toast("尚未连接桌面", "error");
      }
      return;
    }
    if (document.activeElement === keyboardEl) {
      keyboardEl?.blur();
      return;
    }
    if (keyboardEl) {
      keyboardEl.value = "";
      keyboardEl.focus({ preventScroll: true });
    }
  }

  function setModifierBeltOpen(open: boolean) {
    setBeltOpen(open);
  }

  function toggleModifierBelt() {
    setModifierBeltOpen(!beltOpen());
  }

  function openSheet(name: Exclude<SheetName, null>) {
    if (sheet() === name) {
      closeSheets();
      return;
    }
    setSheet(name);
    if (name === "settings") void refreshStatus();
  }

  function closeSheets() {
    setSheet(null);
  }

  async function switchOutput(name: string) {
    if (switchingOutput()) return;
    setSwitchingOutput(name);
    try {
      const result = await api<{ outputs: OutputInfo[] }>("/api/output", {
        method: "POST",
        body: { name },
      });
      const current = status();
      if (current) setStatus({ ...current, outputs: result.outputs, output_error: undefined });
      toast(`已切换到 ${name}`);
      window.setTimeout(() => {
        updateResolution();
        void refreshStatus();
      }, 500);
    } catch (error) {
      if (isUnauthorized(error)) {
        expireSession();
      } else {
        toast(errorMessage(error), "error");
      }
    } finally {
      setSwitchingOutput(null);
    }
  }

  function sendClipboard() {
    if (!rfb || connection() !== "connected") {
      toast("尚未连接桌面", "error");
      return;
    }
    rfb.clipboardPasteFrom(clipboard());
    toast("剪贴板已发送到远端");
  }

  async function readDeviceClipboard() {
    try {
      if (!navigator.clipboard?.readText) throw new Error("unsupported");
      setClipboard(await navigator.clipboard.readText());
    } catch {
      toast("浏览器拒绝读取剪贴板", "error");
    }
  }

  async function copyRemoteClipboard() {
    const text = clipboard();
    if (!text) {
      toast("没有可复制的文本", "error");
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const input = document.createElement("textarea");
      input.value = text;
      input.style.position = "fixed";
      input.style.opacity = "0";
      document.body.append(input);
      input.select();
      document.execCommand("copy");
      input.remove();
    }
    toast("已复制到本机剪贴板");
  }

  async function toggleFullscreen() {
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else if (document.documentElement.requestFullscreen) {
        await document.documentElement.requestFullscreen();
        setMenuHidden(true);
        const orientation = window.screen.orientation as ScreenOrientation & {
          lock?: (mode: string) => Promise<void>;
        };
        orientation.lock?.("landscape").catch(() => undefined);
      } else {
        toast("当前浏览器不支持全屏", "error");
      }
    } catch {
      toast("无法切换全屏", "error");
    }
  }

  async function install() {
    const prompt = installPrompt();
    if (!prompt) return;
    await prompt.prompt().catch(() => undefined);
    await prompt.userChoice.catch(() => undefined);
    setInstallPrompt(null);
  }

  async function requestWakeLock() {
    if (!WAKE_LOCK || wakeLock || document.visibilityState !== "visible") return;
    const nav = navigator as Navigator & {
      wakeLock?: { request(type: "screen"): Promise<WakeLockLike> };
    };
    try {
      wakeLock = (await nav.wakeLock?.request("screen")) ?? null;
      wakeLock?.addEventListener("release", () => {
        wakeLock = null;
      });
    } catch {
      wakeLock = null;
    }
  }

  async function releaseWakeLock() {
    const current = wakeLock;
    wakeLock = null;
    await current?.release().catch(() => undefined);
  }

  function toast(message: string, type: "info" | "error" = "info") {
    const id = ++toastId;
    setToasts([...toasts(), { id, message, type }]);
    const timer = window.setTimeout(() => {
      setToasts(toasts().filter((item) => item.id !== id));
      toastTimers.delete(id);
    }, type === "error" ? 4200 : 2600);
    toastTimers.set(id, timer);
  }

  function registerServiceWorker() {
    if (!import.meta.env.PROD || !("serviceWorker" in navigator) || !window.isSecureContext) return;
    navigator.serviceWorker.register("/sw.js").catch(() => undefined);
  }

  return {
    authenticated,
    loginError,
    loginPending,
    authState,
    authStatusLabel,
    version,
    connection,
    connectionLabel,
    overlay,
    resolution,
    desktopName,
    status,
    outputs,
    currentOutput,
    sheet,
    settings,
    modifiers,
    beltOpen,
    keyboardActive,
    clipboard,
    toasts,
    fullscreen,
    installPrompt,
    switchingOutput,
    buildLabel,
    login,
    logout,
    reconnect,
    connect,
    retry: reconnect,
    attachScreen,
    attachKeyboard,
    startRemote,
    teardownRemote,
    startStatusPolling,
    stopStatusPolling,
    refreshStatus,
    setQuality,
    setCompression,
    viewModeLabel,
    setViewMode,
    cycleViewMode,
    setViewOnly,
    setMenuHidden,
    hideMenu,
    showMenu,
    toggleMenu,
    toggleModifier,
    isModifierActive,
    clearModifiers,
    sendSpecial,
    sendText,
    setKeyboardActive,
    toggleKeyboard,
    toggleModifierBelt,
    setModifierBeltOpen,
    openSheet,
    closeSheets,
    switchOutput,
    sendClipboard,
    readDeviceClipboard,
    copyRemoteClipboard,
    setClipboard,
    toggleFullscreen,
    install,
    toast,
    updateResolution,
  };
}

export type AppController = ReturnType<typeof createAppController>;
