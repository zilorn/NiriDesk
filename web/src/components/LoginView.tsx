import { createSignal, onMount } from "solid-js";
import { useApp } from "../context";
import { BrandMark, Icon } from "./Icons";

export function LoginView() {
  const app = useApp();
  const [password, setPassword] = createSignal("");
  const [showPassword, setShowPassword] = createSignal(false);
  let passwordRef: HTMLInputElement | undefined;

  onMount(() => {
    if (window.matchMedia("(min-width: 821px)").matches) {
      window.setTimeout(() => passwordRef?.focus(), 80);
    }
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (!password() || app.loginPending()) return;
    await app.login(password());
  }

  return (
    <main class="auth-shell">
      <section class="auth-story" aria-labelledby="login-heading">
        <div class="story-glow" aria-hidden="true" />
        <div class="story-grid" aria-hidden="true" />
        <div class="story-content">
          <div class="brand brand-large">
            <BrandMark />
            <span class="brand-text">NiriDesk</span>
          </div>

          <div class="story-copy">
            <p class="eyebrow"><span class="live-dot" />native niri · wayland</p>
            <h1 id="login-heading">
              桌面留在桌面。<br />
              <span>控制权随身。</span>
            </h1>
            <p class="story-lead">
              从手机或任意浏览器进入你的 niri 会话。低延迟画面、原生键鼠注入，不安装客户端。
            </p>
          </div>

          <div class="feature-grid">
            <article class="feature">
              <span class="feature-icon"><Icon name="phone" /></span>
              <h2>为触屏设计</h2>
              <p>点按、拖动、滚轮与虚拟修饰键，横竖屏都能工作。</p>
            </article>
            <article class="feature">
              <span class="feature-icon"><Icon name="layers" /></span>
              <h2>多显示器切换</h2>
              <p>实时选择要捕获的 niri 输出，画面不断线。</p>
            </article>
            <article class="feature">
              <span class="feature-icon"><Icon name="shield" /></span>
              <h2>默认不暴露 VNC</h2>
              <p>wayvnc 只监听回环，浏览器会话经过密码与签名 Cookie 鉴权。</p>
            </article>
          </div>

          <p class="story-foot"><Icon name="lock" /> 建议通过 HTTPS、Tailscale 或内网访问</p>
        </div>
      </section>

      <section class="auth-panel" aria-labelledby="login-form-title">
        <div class="auth-card">
          <div class="mobile-brand brand">
            <BrandMark />
            <span class="brand-text">NiriDesk</span>
          </div>
          <div class="auth-kicker"><span>受保护的会话</span><span class="auth-kicker-line" /></div>
          <h2 id="login-form-title">连接你的桌面</h2>
          <p class="auth-sub">输入部署时设置的访问密码。</p>

          <form id="login-form" autocomplete="on" onSubmit={submit}>
            <label class="field">
              <span class="field-label">访问密码</span>
              <span class="field-control">
                <Icon name="lock" class="field-icon" />
                <input
                  id="password"
                  ref={passwordRef}
                  name="password"
                  type={showPassword() ? "text" : "password"}
                  autocomplete="current-password"
                  placeholder="••••••••"
                  minlength="8"
                  required
                  value={password()}
                  onInput={(event) => setPassword(event.currentTarget.value)}
                />
                <button
                  class="reveal-button"
                  type="button"
                  aria-label={showPassword() ? "隐藏密码" : "显示密码"}
                  onClick={() => setShowPassword(!showPassword())}
                >
                  <Icon name={showPassword() ? "eye-off" : "eye"} />
                </button>
              </span>
            </label>
            <p class="form-error" role="alert" aria-live="polite">{app.loginError()}</p>
            <button
              class={`button button-primary button-large${app.loginPending() ? " loading" : ""}`}
              type="submit"
              disabled={app.loginPending()}
            >
              <span class="button-label">进入桌面</span>
              <Icon name="chevron" class="button-icon" />
              <span class="button-spinner" aria-hidden="true" />
            </button>
          </form>

          <div class="auth-meta">
            <div class="auth-meta-item"><Icon name="shield" /><span>HttpOnly 签名会话</span></div>
            <div class="auth-meta-item"><Icon name="wifi" /><span>自适应画质</span></div>
          </div>

          <div class="auth-status" data-state={app.authState()}>
            <span class="status-dot" />
            <span>{app.authStatusLabel()}</span>
          </div>
          <p class="auth-version">NiriDesk {app.version()}</p>
        </div>
      </section>
    </main>
  );
}
