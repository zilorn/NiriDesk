import { Show, createEffect, onCleanup, onMount } from "solid-js";
import { useApp } from "../context";
import type { SpecialName } from "../lib/keys";
import { ClipboardSheet } from "./ClipboardSheet";
import { BrandMark, Icon } from "./Icons";
import { ModifierBelt } from "./ModifierBelt";
import { SettingsSheet } from "./SettingsSheet";
import { Toasts } from "./Toasts";

const SPECIAL_BY_KEY: Record<string, SpecialName> = {
  Escape: "escape",
  Tab: "tab",
  Enter: "enter",
  Backspace: "backspace",
  ArrowLeft: "left",
  ArrowRight: "right",
  ArrowUp: "up",
  ArrowDown: "down",
};

export function RemoteView() {
  const app = useApp();
  let screenElement: HTMLDivElement | undefined;
  let keyboardInput: HTMLInputElement | undefined;
  let composing = false;

  onMount(() => {
    if (screenElement) app.attachScreen(screenElement);
    if (keyboardInput) app.attachKeyboard(keyboardInput);
    app.startRemote();

    document.addEventListener("keydown", handleDocumentKeydown);
    onCleanup(() => {
      document.removeEventListener("keydown", handleDocumentKeydown);
      app.attachScreen(null);
      app.attachKeyboard(null);
      app.teardownRemote();
      document.body.classList.remove("sheet-open");
    });
  });

  createEffect(() => {
    document.body.classList.toggle("sheet-open", app.sheet() !== null);
  });

  function handleDocumentKeydown(event: KeyboardEvent) {
    if (!app.sheet()) return;
    if (event.key === "Escape") {
      event.preventDefault();
      app.closeSheets();
      return;
    }
    if (event.key !== "Tab") return;
    const id = app.sheet() === "settings" ? "settings-sheet" : "clipboard-sheet";
    const sheet = document.getElementById(id);
    if (!sheet) return;
    const focusable = Array.from(
      sheet.querySelectorAll<HTMLElement>("button, input, textarea, [tabindex]:not([tabindex='-1'])"),
    ).filter((element) => !element.hasAttribute("disabled") && element.offsetParent !== null);
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  function handleKeyboardInput(event: InputEvent & { currentTarget: HTMLInputElement }) {
    if (composing || event.isComposing) return;
    const value = event.currentTarget.value;
    if (value) app.sendText(value);
    event.currentTarget.value = "";
  }

  function handleKeyboardKeydown(event: KeyboardEvent & { currentTarget: HTMLInputElement }) {
    if (event.isComposing) return;
    const special = SPECIAL_BY_KEY[event.key];
    if (special) {
      event.preventDefault();
      app.sendSpecial(special);
    }
  }

  return (
    <>
      <div
        class={`app${app.beltOpen() ? " modifier-open" : ""}${app.settings.menuHidden ? " chrome-hidden" : ""}`}
        id="app-view"
      >
        <header class="topbar">
          <div class="brand brand-compact">
            <BrandMark />
            <span class="brand-copy">
              <span class="brand-text">NiriDesk</span>
              <span class="brand-caption">{app.desktopName()}</span>
            </span>
          </div>

          <div class="connection-pill" data-state={app.connection()} title="连接状态">
            <span class="connection-dot" />
            <span>{app.connectionLabel()}</span>
          </div>

          <div class="top-actions">
            <button class="icon-button desktop-action" type="button" aria-label="键盘" title="键盘" onClick={app.toggleKeyboard}>
              <Icon name="keyboard" />
            </button>
            <button class="icon-button desktop-action" type="button" aria-label="剪贴板" title="剪贴板" onClick={() => app.openSheet("clipboard")}>
              <Icon name="clipboard" />
            </button>
            <button
              class={`icon-button desktop-action${app.settings.viewMode === "fit" ? " active" : ""}`}
              type="button"
              aria-label={`切换缩放模式，当前${app.viewModeLabel()}`}
              title={`缩放：${app.viewModeLabel()}`}
              onClick={app.cycleViewMode}
            >
              <Icon name="fit" />
            </button>
            <button class="icon-button" type="button" aria-label={app.fullscreen() ? "退出全屏" : "全屏"} title="全屏" onClick={app.toggleFullscreen}>
              <Icon name={app.fullscreen() ? "minimize" : "maximize"} />
            </button>
            <button class="icon-button" type="button" aria-label="设置" title="设置" onClick={() => app.openSheet("settings")}>
              <Icon name="sliders" />
            </button>
          </div>
        </header>

        <main class="workspace">
          <section class="stage" aria-label="远程桌面">
            <div class="screen" id="screen" ref={(element) => { screenElement = element; }} />

            <Show when={app.overlay()}>
              {(overlay) => (
                <div class="stage-overlay" data-state={overlay().kind}>
                  <div class="overlay-card">
                    <div class="overlay-visual">
                      <span class="orbit orbit-one" />
                      <span class="orbit orbit-two" />
                      <span class="overlay-glyph"><Icon name="monitor" /></span>
                    </div>
                    <p class="overlay-kicker">niri / wayland</p>
                    <h2>{overlay().title}</h2>
                    <p>{overlay().detail}</p>
                    <Show when={overlay().retry}>
                      <button class="button button-primary" type="button" onClick={app.retry}>
                        <Icon name="refresh" class="button-icon" />
                        <span>重新连接</span>
                      </button>
                    </Show>
                  </div>
                </div>
              )}
            </Show>

            <Show when={app.connection() === "connected"}>
              <div class="session-pill">
                <span class="session-dot" />
                <span>{app.resolution()}</span>
                <span class="session-separator" />
                <span>{app.currentOutput()?.name ?? "—"}</span>
              </div>
            </Show>

            <Toasts />
          </section>

          <Show when={app.beltOpen()}>
            <ModifierBelt />
          </Show>
        </main>

        <nav class="mobile-toolbar" aria-label="远程桌面控制">
          <button
            class={`tool-button${app.keyboardActive() ? " active" : ""}`}
            type="button"
            aria-pressed={app.keyboardActive()}
            onClick={app.toggleKeyboard}
          >
            <Icon name="keyboard" /><span>键盘</span>
          </button>
          <button
            class={`tool-button${app.beltOpen() ? " active" : ""}`}
            type="button"
            aria-pressed={app.beltOpen()}
            onClick={app.toggleModifierBelt}
          >
            <Icon name="command" /><span>修饰键</span>
          </button>
          <button
            class={`tool-button${app.settings.viewMode === "fill" ? " active" : ""}`}
            type="button"
            aria-label={`切换缩放模式，当前${app.viewModeLabel()}`}
            onClick={app.cycleViewMode}
          >
            <Icon name="fit" /><span>{app.viewModeLabel()}</span>
          </button>
          <button class="tool-button" type="button" onClick={() => app.openSheet("clipboard")}>
            <Icon name="clipboard" /><span>剪贴板</span>
          </button>
          <button class="tool-button" type="button" onClick={() => app.openSheet("settings")}>
            <Icon name="sliders" /><span>设置</span>
          </button>
        </nav>

        <button
          class="menu-toggle"
          type="button"
          aria-label={app.settings.menuHidden ? "显示菜单" : "隐藏菜单"}
          aria-expanded={!app.settings.menuHidden}
          title={app.settings.menuHidden ? "显示菜单" : "隐藏菜单"}
          onClick={app.toggleMenu}
        >
          <Icon name={app.settings.menuHidden ? "menu" : "chevron-down"} />
        </button>

        <input
          ref={(element) => { keyboardInput = element; }}
          class="mobile-input"
          type="text"
          inputmode="text"
          autocomplete="off"
          autocorrect="off"
          autocapitalize="off"
          spellcheck={false}
          aria-hidden="true"
          tabindex="-1"
          disabled={app.settings.viewOnly}
          onFocus={() => app.setKeyboardActive(true)}
          onBlur={() => app.setKeyboardActive(false)}
          onInput={handleKeyboardInput}
          onKeyDown={handleKeyboardKeydown}
          onCompositionStart={() => { composing = true; }}
          onCompositionEnd={(event) => {
            composing = false;
            app.sendText(event.data || event.currentTarget.value);
            event.currentTarget.value = "";
          }}
        />
      </div>

      <Show when={app.sheet()}>
        <div class="backdrop" onClick={app.closeSheets} />
      </Show>
      <SettingsSheet />
      <ClipboardSheet />
    </>
  );
}
