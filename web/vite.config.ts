import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development the Go server (CAIRN_ADDR, :8080) answers /api.
const BACKEND = process.env.CAIRN_BACKEND ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  build: { assetsInlineLimit: 0 },
  server: {
    proxy: {
      "/api": { target: BACKEND },
      "/healthz": { target: BACKEND },
    },
  },
});
