import { defineConfig, devices } from "@playwright/test";

// Every port is overridable: the web server runs on 3000, 3001, or 3002 (any
// PLAYWRIGHT_WEB_PORT) and the API it pairs with runs on 8080 or 8081 (any
// PLAYWRIGHT_API_BASE_URL). The stack launcher receives both explicitly, so
// no combination needs other edits.
const webPort = Number(process.env.PLAYWRIGHT_WEB_PORT ?? 3001);
const apiPort = Number(process.env.PLAYWRIGHT_API_PORT ?? 8081);
const apiBaseURL = process.env.PLAYWRIGHT_API_BASE_URL ?? `http://localhost:${apiPort}`;
// A cold production build of the dashboard can take minutes on a slow VM.
const webServerTimeout = Number(process.env.PLAYWRIGHT_WEBSERVER_TIMEOUT ?? 480_000);

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
    // run-stack.sh also brings the API (:8081 — database, migrations, ready
    // probe) up with the web server, so the suite never has to be pointed at
    // someone else's api. An already-ready API is reused untouched.
    command: `bash ./e2e/run-stack.sh ${apiBaseURL} ${webPort}`,
    url: `http://127.0.0.1:${webPort}`,
    // Opt-in reuse: prestart the server with ./e2e/run-web-server.sh and set
    // PLAYWRIGHT_REUSE_WEB_SERVER=1 to skip the rebuild per run.
    reuseExistingServer: process.env.PLAYWRIGHT_REUSE_WEB_SERVER === "1",
    timeout: webServerTimeout,
    stdout: "pipe",
    stderr: "pipe",
  },
});
