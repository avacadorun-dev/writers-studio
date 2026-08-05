import { defineConfig, devices } from "@playwright/test";

const audioFile = process.env.REAL_STT_AUDIO_FILE;
if (!audioFile) {
  throw new Error("REAL_STT_AUDIO_FILE is required for the real STT browser test");
}

export default defineConfig({
  testDir: "./tests/real-stt",
  outputDir: "./test-results/real-stt",
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  workers: 1,
  timeout: 120_000,
  expect: {
    timeout: 35_000,
  },
  reporter: [["line"], ["html", { outputFolder: "playwright-report-real-stt", open: "never" }]],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://127.0.0.1:8080",
    locale: "ru-RU",
    timezoneId: "Asia/Krasnoyarsk",
    trace: "on",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    launchOptions: {
      args: [
        "--use-fake-device-for-media-stream",
        "--use-fake-ui-for-media-stream",
        `--use-file-for-fake-audio-capture=${audioFile}`,
        "--autoplay-policy=no-user-gesture-required",
      ],
    },
  },
  projects: [
    {
      name: "chromium-real-stt",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 1440, height: 1024 },
      },
    },
  ],
});
