import { For, Show, createEffect } from "solid-js";
import { useApp } from "../context";
import { Icon } from "./Icons";

interface StatusLine {
  text: string;
  mode: "ok" | "bad" | "warn";
}

export function SettingsSheet() {
  const app = useApp();
  let sheetElement: HTMLElement | undefined;

  createEffect(() => {
    if (sheetElement) sheetElement.inert = app.sheet() !== "settings";
  });

  createEffect(() => {
    if (app.sheet() === "settings") {
      window.setTimeout(() => {
        if (app.sheet() === "settings") {
          sheetElement?.querySelector<HTMLButtonElement>("[data-close-sheet]")?.focus();
        }
      }, 220);
    }
  });

  const wayvncLabel = () => {
    const current = app.status();
    const state = current?.vnc.wayvnc;
    if (!state) return current?.vnc.reachable ? "可用" : "不可用";
    const version = state.version ? ` ${state.version}` : "";
    if (!state.managed) return current?.vnc.reachable ? `外部${version}` : `外部不可达${version}`;
    return state.running ? `运行中${version}` : `未运行${version}`;
  };

  const audioLine = (): StatusLine => {
    const current = app.status();
    if (!current) return { text: "正在读取音频状态…", mode: "warn" };
    if (!current.audio.enabled) return { text: "服务端已关闭音频采集", mode: "warn" };
    if (!current.audio.available) {
      return { text: current.audio.error || "音频采集不可用，请检查 PulseAudio/PipeWire 与 parec", mode: "bad" };
    }
    switch (app.audioState()) {
      case "playing": return { text: "正在播放远端系统声音 · 48 kHz 立体声", mode: "ok" };
      case "starting": return { text: "正在建立音频通道…", mode: "warn" };
      case "error": return { text: app.audioError() || "音频连接异常", mode: "bad" };
      default: return { text: "声音未开启，点按开关播放远端音频", mode: "warn" };
    }
  };

  const privacyLine = (): StatusLine => {
    const current = app.status();
    if (!current) return { text: "正在读取防窥状态…", mode: "warn" };
    if (!current.privacy.supported) {
      return { text: current.privacy.last_error || "当前合成器不支持 Wayland gamma control，无法启用防窥模式", mode: "warn" };
    }
    if (current.privacy.last_error) {
      return { text: current.privacy.last_error, mode: "bad" };
    }
    if (current.privacy.enabled) {
      return { text: "防窥模式已开启：物理显示器已全黑，远程画面继续传输", mode: "ok" };
    }
    return { text: "防窥模式已关闭", mode: "warn" };
  };

  const statusLines = (): StatusLine[] => {
    const current = app.status();
    if (!current) return [{ text: "正在读取服务状态…", mode: "warn" }];
    const lines: StatusLine[] = [];
    lines.push({
      text: current.wayland.socket_present ? `Wayland 会话 ${current.wayland.display}` : "Wayland 会话不可见",
      mode: current.wayland.socket_present ? "ok" : "bad",
    });
    lines.push({
      text: current.vnc.reachable ? "VNC 内部通道可达" : "VNC 内部通道不可达",
      mode: current.vnc.reachable ? "ok" : "bad",
    });
    lines.push({
      text: current.outputs.length ? `${current.outputs.length} 个 niri 输出可用` : "未发现可捕获输出",
      mode: current.outputs.length ? "ok" : "warn",
    });
    if (current.output_error) lines.push({ text: current.output_error, mode: "warn" });
    if (current.vnc.wayvnc.last_error) lines.push({ text: current.vnc.wayvnc.last_error, mode: "warn" });
    return lines;
  };

  return (
    <aside
      ref={(element) => { sheetElement = element; }}
      class={`sheet${app.sheet() === "settings" ? " open" : ""}`}
      id="settings-sheet"
      aria-labelledby="settings-title"
      aria-hidden={app.sheet() !== "settings"}
    >
      <header class="sheet-header">
        <div>
          <p class="sheet-kicker">session control</p>
          <h2 id="settings-title">设置</h2>
        </div>
        <button class="icon-button" data-close-sheet type="button" aria-label="关闭设置" onClick={app.closeSheets}>
          <Icon name="x" />
        </button>
      </header>
      <div class="sheet-body">
        <section class="settings-section">
          <h3>画面</h3>
          <div class="setting-row setting-row-stack">
            <div class="setting-copy">
              <label for="quality">画质</label>
              <p>网络较差时降低画质可以减少卡顿。</p>
            </div>
            <div class="range-row">
              <input
                id="quality"
                class="range"
                type="range"
                min="1"
                max="9"
                step="1"
                value={app.settings.quality}
                onInput={(event) => app.setQuality(Number(event.currentTarget.value))}
              />
              <output for="quality">{app.settings.quality}</output>
            </div>
          </div>
          <div class="setting-row setting-row-stack">
            <div class="setting-copy">
              <label for="compression">压缩</label>
              <p>更高压缩更省流量，但会增加服务端负载。</p>
            </div>
            <div class="range-row">
              <input
                id="compression"
                class="range"
                type="range"
                min="0"
                max="9"
                step="1"
                value={app.settings.compression}
                onInput={(event) => app.setCompression(Number(event.currentTarget.value))}
              />
              <output for="compression">{app.settings.compression}</output>
            </div>
          </div>
          <div class="setting-row setting-row-stack">
            <div class="setting-copy">
              <span>缩放模式</span>
              <p>「铺满」按屏幕裁切画面边缘，拖动即可平移；「1:1」按原始像素拖动查看。</p>
            </div>
            <div class="segmented" role="group" aria-label="缩放模式">
              <button
                class={app.settings.viewMode === "fit" ? "active" : ""}
                type="button"
                aria-pressed={app.settings.viewMode === "fit"}
                onClick={() => app.setViewMode("fit")}
              >
                适应
              </button>
              <button
                class={app.settings.viewMode === "fill" ? "active" : ""}
                type="button"
                aria-pressed={app.settings.viewMode === "fill"}
                onClick={() => app.setViewMode("fill")}
              >
                铺满
              </button>
              <button
                class={app.settings.viewMode === "actual" ? "active" : ""}
                type="button"
                aria-pressed={app.settings.viewMode === "actual"}
                onClick={() => app.setViewMode("actual")}
              >
                1:1
              </button>
            </div>
          </div>
          <label class="setting-row">
            <span class="setting-copy"><span>只看不控</span><p>禁止发送键盘与鼠标事件。</p></span>
            <span class="switch">
              <input
                type="checkbox"
                checked={app.settings.viewOnly}
                onChange={(event) => app.setViewOnly(event.currentTarget.checked)}
              />
              <span class="switch-track" />
            </span>
          </label>
        </section>

        <section class="settings-section mobile-setting">
          <h3>界面</h3>
          <label class="setting-row mobile-setting">
            <span class="setting-copy">
              <span>隐藏控制菜单</span>
              <p>菜单浮层隐藏后画面占满手机屏幕，点屏幕右侧悬浮球可恢复。</p>
            </span>
            <span class="switch">
              <input
                type="checkbox"
                checked={app.settings.menuHidden}
                onChange={(event) => app.setMenuHidden(event.currentTarget.checked)}
              />
              <span class="switch-track" />
            </span>
          </label>
        </section>

        <section class="settings-section">
          <h3>声音</h3>
          <label class="setting-row">
            <span class="setting-copy">
              <span>同步远端音频</span>
              <p>播放 niri 会话的系统声音；需要点击开关以通过浏览器的自动播放限制。</p>
            </span>
            <span class="switch">
              <input
                type="checkbox"
                checked={app.audioActive()}
                disabled={app.audioState() === "starting" || app.connection() !== "connected" || (!app.audioActive() && !app.audioAvailable())}
                onChange={() => void app.toggleAudio()}
              />
              <span class="switch-track" />
            </span>
          </label>
          <div class="setting-row setting-row-stack">
            <div class="setting-copy">
              <label for="audio-delay">音画同步延迟</label>
              <p>声音在网络中先缓冲再播放。若口型落后于画面，增大延迟；数值越低延迟越小。</p>
            </div>
            <div class="range-row">
              <input
                id="audio-delay"
                class="range"
                type="range"
                min="40"
                max="500"
                step="10"
                value={app.settings.audioDelay}
                onInput={(event) => app.setAudioDelay(Number(event.currentTarget.value))}
              />
              <output class="range-value" for="audio-delay">{app.settings.audioDelay} ms</output>
            </div>
          </div>
          <div class="status-lines">
            <div class={`status-line ${audioLine().mode}`}>
              <span class="status-dot" />
              <span>{audioLine().text}</span>
            </div>
          </div>
        </section>

        <section class="settings-section">
          <h3>隐私</h3>
          <label class="setting-row">
            <span class="setting-copy">
              <span>防窥模式</span>
              <p>远程操作期间关闭物理显示器。最后一个远程画面会话断开、关闭此开关或服务退出时自动恢复。</p>
            </span>
            <span class="switch">
              <input
                type="checkbox"
                checked={app.privacyEnabled()}
                disabled={!app.privacySupported() || app.privacyPending()}
                onChange={(event) => void app.setPrivacy(event.currentTarget.checked)}
              />
              <span class="switch-track" />
            </span>
          </label>
          <div class="status-lines">
            <div class={`status-line ${privacyLine().mode}`}>
              <span class="status-dot" />
              <span>{privacyLine().text}</span>
            </div>
          </div>
          <p class="setting-note">防窥通过 Wayland gamma control 将物理输出压成全黑，screencopy 仍取得正常画面；远程键鼠不会唤醒或恢复显示内容。关闭开关、最后一个远程画面会话断开或服务退出时会恢复。需要合成器支持 wlr-gamma-control。</p>
        </section>

        <section class="settings-section">
          <div class="section-heading">
            <h3>显示器输出</h3>
            <span class="section-badge">{app.outputs().length} 个</span>
          </div>
          <div class="output-list">
            <Show
              when={app.outputs().length > 0}
              fallback={<div class="settings-empty">没有读取到 niri 输出。请确认 wayvnc 已连接到当前 Wayland 会话。</div>}
            >
              <For each={app.outputs()}>
                {(output) => (
                  <button
                    class={`output-item${output.captured ? " captured" : ""}${app.switchingOutput() === output.name ? " switching" : ""}`}
                    type="button"
                    disabled={output.captured || Boolean(app.switchingOutput())}
                    onClick={() => app.switchOutput(output.name)}
                  >
                    <span class="output-icon"><Icon name="monitor" /></span>
                    <span class="output-copy">
                      <strong>{output.name}</strong>
                      <span>{output.width || "?"} × {output.height || "?"} · {output.description || "niri output"}</span>
                    </span>
                    <span class="output-state"><Icon name="check" /><span>正在捕获</span></span>
                    <span class="output-switching">切换中…</span>
                  </button>
                )}
              </For>
            </Show>
          </div>
        </section>

        <section class="settings-section">
          <h3>会话</h3>
          <div class="stat-grid">
            <div class="stat"><span>画面尺寸</span><strong>{app.resolution()}</strong></div>
            <div class="stat"><span>捕获帧率上限</span><strong>{app.status()?.max_fps ? `${app.status()?.max_fps} fps` : "—"}</strong></div>
            <div class="stat"><span>活动连接</span><strong>{app.status() ? `${app.status()?.active_clients} / ${app.status()?.max_clients}` : "—"}</strong></div>
            <div class="stat"><span>wayvnc</span><strong>{wayvncLabel()}</strong></div>
          </div>
          <div class="status-lines">
            <For each={statusLines()}>
              {(line) => (
                <div class={`status-line ${line.mode}`}>
                  <span class="status-dot" />
                  <span>{line.text}</span>
                </div>
              )}
            </For>
          </div>
        </section>

        <Show when={app.installPrompt()}>
          <section class="settings-section">
            <button class="setting-row install-row" type="button" onClick={app.install}>
              <span class="setting-copy"><span>安装到主屏幕</span><p>以独立窗口全屏运行，体验更接近原生。</p></span>
              <Icon name="download" />
            </button>
          </section>
        </Show>

        <section class="settings-section">
          <h3>操作</h3>
          <div class="sheet-actions">
            <button class="button button-secondary" type="button" onClick={app.reconnect}>
              <Icon name="refresh" class="button-icon" /><span>重新连接</span>
            </button>
            <button class="button button-danger" type="button" onClick={app.logout}>
              <Icon name="logout" class="button-icon" /><span>退出登录</span>
            </button>
          </div>
        </section>

        <footer class="sheet-footer">
          <span class="brand-mini">NiriDesk</span>
          <span>{app.buildLabel()}</span>
        </footer>
      </div>
    </aside>
  );
}
