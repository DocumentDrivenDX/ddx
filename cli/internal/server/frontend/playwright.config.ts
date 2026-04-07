import { defineConfig } from '@playwright/test'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const cliRoot = resolve(__dirname, '..', '..', '..')

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: {
    baseURL: 'http://127.0.0.1:18080',
    headless: true,
  },
  webServer: {
    command: `${cliRoot}/build/ddx server --port 18080`,
    cwd: cliRoot,
    port: 18080,
    reuseExistingServer: true,
    timeout: 10000,
  },
})
