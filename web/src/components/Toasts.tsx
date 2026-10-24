import { For } from "solid-js";
import { useApp } from "../context";

export function Toasts() {
  const app = useApp();
  return (
    <div class="toast-region" aria-live="polite">
      <For each={app.toasts()}>
        {(toast) => <div class="toast" data-type={toast.type}>{toast.message}</div>}
      </For>
    </div>
  );
}
