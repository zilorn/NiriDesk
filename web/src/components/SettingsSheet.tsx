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
          <label class="setting-row">
            <span class="setting-copy"><span>适应窗口</span><p>自动缩放，适合手机竖屏查看。</p></span>
            <span class="switch">
              <input
                type="checkbox"
                checked={app.settings.fit}
                onChange={(event) => app.setFit(event.currentTarget.checked)}
              />
              <span class="switch-track" />
            </span>
          </label>
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
