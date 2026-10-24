import { createEffect } from "solid-js";
import { useApp } from "../context";
import { Icon } from "./Icons";

export function ClipboardSheet() {
  const app = useApp();
  let sheetElement: HTMLElement | undefined;

  createEffect(() => {
    if (sheetElement) sheetElement.inert = app.sheet() !== "clipboard";
  });

  return (
    <aside
      ref={(element) => { sheetElement = element; }}
      class={`sheet${app.sheet() === "clipboard" ? " open" : ""}`}
      id="clipboard-sheet"
      aria-labelledby="clipboard-title"
      aria-hidden={app.sheet() !== "clipboard"}
    >
      <header class="sheet-header">
        <div>
          <p class="sheet-kicker">clipboard bridge</p>
          <h2 id="clipboard-title">剪贴板</h2>
        </div>
        <button class="icon-button" data-close-sheet type="button" aria-label="关闭剪贴板" onClick={app.closeSheets}>
          <Icon name="x" />
        </button>
      </header>
      <div class="sheet-body">
        <p class="sheet-intro">在本机与 niri 会话之间传递文本。浏览器剪贴板权限可能需要在站点设置中允许。</p>
        <label class="field clipboard-field">
          <span class="field-label">文本</span>
          <textarea
            rows="8"
            placeholder="输入文本，或从远端剪贴板接收…"
            value={app.clipboard()}
            onInput={(event) => app.setClipboard(event.currentTarget.value)}
          />
        </label>
        <div class="clipboard-actions">
          <button class="button button-primary" type="button" onClick={app.sendClipboard}>
            <Icon name="send" class="button-icon" /><span>发送到远端</span>
          </button>
          <button class="button button-secondary" type="button" onClick={() => void app.readDeviceClipboard()}>
            <Icon name="download" class="button-icon" /><span>读取本机</span>
          </button>
          <button class="button button-secondary" type="button" onClick={() => void app.copyRemoteClipboard()}>
            <Icon name="copy" class="button-icon" /><span>复制远端内容</span>
          </button>
        </div>
        <div class="clipboard-hint"><kbd>Ctrl</kbd><span>+</span><kbd>V</kbd><span>在远端应用中粘贴</span></div>
      </div>
    </aside>
  );
}
