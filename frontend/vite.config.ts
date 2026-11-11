import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
export default defineConfig({
  plugins: [vue()],
  server: {
    allowedHosts: [".iximiuz.com"],
    proxy: { "/api": process.env.API_PROXY_TARGET || "http://127.0.0.1:8080" },
    watch: { usePolling: process.env.VITE_USE_POLLING === "true" },
  },
});
