import { createContext, useContext } from "solid-js";
import type { AppController } from "./createAppController";

export const AppContext = createContext<AppController>();

export function useApp(): AppController {
  const app = useContext(AppContext);
  if (!app) throw new Error("AppContext is not available");
  return app;
}
