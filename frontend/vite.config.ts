import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const proxyTarget = env.VITE_API_PROXY_TARGET;
  const base = env.VITE_BASE_PATH || "/ui/";

  return {
    base,
    plugins: [react()],
    server: {
      port: Number(env.VITE_PORT || 5173),
      proxy: proxyTarget
        ? {
            "/api": {
              target: proxyTarget,
              changeOrigin: true,
            },
            "/healthz": {
              target: proxyTarget,
              changeOrigin: true,
            },
            "/readyz": {
              target: proxyTarget,
              changeOrigin: true,
            },
            "/service-registry": {
              target: proxyTarget,
              changeOrigin: true,
            },
            "/outbox": {
              target: proxyTarget,
              changeOrigin: true,
            },
            "/projections": {
              target: proxyTarget,
              changeOrigin: true,
            },
            "/openapi.json": {
              target: proxyTarget,
              changeOrigin: true,
            },
          }
        : undefined,
    },
    test: {
      environment: "jsdom",
      include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
      setupFiles: "./src/test/setup.ts",
    },
  };
});
