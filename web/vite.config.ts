import { defineConfig } from "vite";
import solid from "vite-plugin-solid";

export default defineConfig({
  plugins: [solid()],
  build: {
    target: "es2022",
    outDir: "dist",
    assetsDir: "assets",
  },
  server: {
    host: true,
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
      },
      "/websocket": {
        target: "ws://127.0.0.1:8080",
        ws: true,
      },
    },
  },
});
