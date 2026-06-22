import { defineConfig, devices } from "@playwright/test";

const usePreview = process.env.NEXUSPAAS_E2E_USE_PREVIEW === "1";
const baseURL = process.env.NEXUSPAAS_E2E_BASE_URL || (usePreview ? "http://127.0.0.1:4173" : "http://127.0.0.1:18080");

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 60_000,
  expect: {
    timeout: 10_000,
  },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL,
    trace: "off",
    screenshot: "off",
    video: "off",
  },
  ...(usePreview
    ? {
        webServer: {
          command: "npm run preview -- --port 4173",
          url: "http://127.0.0.1:4173",
          reuseExistingServer: !process.env.CI,
          timeout: 120_000,
        },
      }
    : {}),
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
