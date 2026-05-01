import { defineConfig } from '@playwright/test'

const webServerPort = Number(process.env.PLAYWRIGHT_WEB_SERVER_PORT ?? '1313')
const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? `http://127.0.0.1:${webServerPort}`
const webServerCommand =
  process.env.PLAYWRIGHT_WEB_SERVER_COMMAND ??
  `hugo server --port ${webServerPort} --baseURL ${baseURL}/ --appendPort=false`

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: {
    baseURL,
    headless: true,
  },
  webServer: {
    command: webServerCommand,
    port: webServerPort,
    reuseExistingServer: true,
    timeout: 10000,
  },
})
