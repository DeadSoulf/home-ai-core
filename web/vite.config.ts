import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    proxy: {
      "/api": { target: "https://127.0.0.1:82", secure: true, ws: true },
      "/health": { target: "https://127.0.0.1:82", secure: true, ws: true },
    },
  },
});
