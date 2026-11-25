import { defineConfig, devices } from "@playwright/test";

const coverage = process.env.UI_COVERAGE === "1";
const frontendDir = coverage ? ".vite/coverage-dist" : "dist";
// Keep individual statements in the source maps used to report Vue and TypeScript coverage.
const buildFrontend = coverage
  ? `npm run build -- --sourcemap --minify=false --outDir ${frontendDir} --emptyOutDir`
  : "npm run build";

export default defineConfig({
  globalTeardown: coverage ? "./tests/coverage-report.ts" : undefined,
  testDir: "./tests",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  use: {
    baseURL: "http://127.0.0.1:18080",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command:
      `${buildFrontend} && mkdir -p ../.tmp && go build -o ../.tmp/ui-test-server ../internal/testutil/cmd/testserver && exec ../.tmp/ui-test-server -frontend ${frontendDir}`,
    url: "http://127.0.0.1:18080/api/health",
    reuseExistingServer: false,
    timeout: 120_000,
    gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
  },
});
