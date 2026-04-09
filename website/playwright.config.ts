import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: {
    baseURL: 'http://127.0.0.1:1313',
    headless: true,
  },
  webServer: {
    command: 'hugo server --port 1313 --baseURL http://127.0.0.1:1313/ --appendPort=false',
    port: 1313,
    reuseExistingServer: true,
    timeout: 10000,
  },
})
