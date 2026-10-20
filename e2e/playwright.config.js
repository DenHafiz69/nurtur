const { defineConfig } = require("@playwright/test");

module.exports = defineConfig({
  testDir: "./tests",
  timeout: 45_000,
  fullyParallel: false,
  workers: 1,
  reporter: [["list"]],
  use: {
    baseURL: "http://127.0.0.1:8097",
    viewport: { width: 390, height: 844 },
    trace: "on-first-retry"
  },
  webServer: {
    command:
      "mkdir -p .e2e && rm -f .e2e/nurtur.db .e2e/nurtur.db-wal .e2e/nurtur.db-shm && NURTUR_LOCAL_ONLY=true NURTUR_DB=.e2e/nurtur.db NURTUR_ADDR=127.0.0.1:8097 NURTUR_TZ=Asia/Kuala_Lumpur NURTUR_BABY_NAME=Pip go run ./cmd/nurtur",
    url: "http://127.0.0.1:8097/healthz",
    cwd: "..",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  }
});
