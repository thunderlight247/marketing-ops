import { defineConfig, devices } from "@playwright/test";

const webPort = Number(process.env.PLAYWRIGHT_WEB_PORT ?? 3001);
const apiBaseURL = process.env.PLAYWRIGHT_API_BASE_URL ?? "http://localhost:8081";

export default defineConfig({
  testDir: "./e2e",
  outputDir: `test-results-${webPort}`,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 120_000,
  reporter: "list",
  use: {
    baseURL: `http://127.0.0.1:${webPort}`,
    actionTimeout: 10_000,
    navigationTimeout: 20_000,
    ...devices["Desktop Chrome"],
    launchOptions: {
      executablePath:
        process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ??
        (process.env.CI ? undefined : "/usr/bin/chromium"),
      // This VM has no usable GPU: without --disable-gpu the GPU-process path
      // stalls or crashes, which starves requestAnimationFrame and hangs
      // Playwright's rAF-driven actionability ("stable") checks forever.
      args: ["--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu"],
    },
    trace: "retain-on-failure",
  },
  webServer: {
    command: `bash ./e2e/run-web-server.sh ${apiBaseURL} ${webPort}`,
    url: `http://127.0.0.1:${webPort}`,
    // Opt-in reuse: prestart the server with ./e2e/run-web-server.sh and set
    // PLAYWRIGHT_REUSE_WEB_SERVER=1 to skip the rebuild per run.
    reuseExistingServer: process.env.PLAYWRIGHT_REUSE_WEB_SERVER === "1",
    timeout: 180_000,
    stdout: "pipe",
    stderr: "pipe",
  },
});
