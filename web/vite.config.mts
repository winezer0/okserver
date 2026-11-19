import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

const target = process.env.OKSERVER_WEBUI_API_TARGET || "http://127.0.0.1:28080";
const proxy = Object.fromEntries(
  ["/upload", "/download", "/api", "/healthz", "/readyz", "/metrics", "/favicon.ico"].map(
    (path) => [path, { target, changeOrigin: false, timeout: 0, proxyTimeout: 0 }],
  ),
);
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: { host: "127.0.0.1", port: 28081, strictPort: true, proxy },
  build: {
    outDir: fileURLToPath(new URL("../internal/webui/assets", import.meta.url)),
    emptyOutDir: true,
    cssCodeSplit: false,
    rolldownOptions: {
      output: {
        entryFileNames: "app.js",
        chunkFileNames: "assets/[name]-[hash].js",
        assetFileNames: "app.[ext]",
      },
    },
  },
  test: {
    include: ["src/**/*.test.{ts,tsx}"],
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
    restoreMocks: true,
  },
});
