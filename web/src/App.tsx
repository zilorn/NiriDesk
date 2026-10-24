import { Match, Switch } from "solid-js";
import { AppContext } from "./context";
import { createAppController } from "./createAppController";
import { IconSprite, BrandMark } from "./components/Icons";
import { LoginView } from "./components/LoginView";
import { RemoteView } from "./components/RemoteView";

function BootScreen() {
  return (
    <main class="boot-screen">
      <div class="brand brand-large">
        <BrandMark />
        <span class="brand-text">NiriDesk</span>
      </div>
      <span class="boot-spinner" aria-label="正在加载" />
      <p>正在检查会话…</p>
    </main>
  );
}

export default function App() {
  const app = createAppController();
  return (
    <AppContext.Provider value={app}>
      <IconSprite />
      <Switch fallback={<BootScreen />}>
        <Match when={app.authenticated() === false}><LoginView /></Match>
        <Match when={app.authenticated() === true}><RemoteView /></Match>
      </Switch>
    </AppContext.Provider>
  );
}
