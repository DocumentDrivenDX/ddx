import { defineConfig } from '@playwright/test'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const cliRoot = resolve(__dirname, '..', '..', '..')

export default defineConfig({
  testDir: './e2e',
  testMatch: 'demo-recording.spec.ts',
  timeout: 60000,
  use: {
    baseURL: 'http://127.0.0.1:18080',
    headless: true,
    viewport: { width: 1280, height: 720 },
    video: {
      mode: 'on',
      size: { width: 1280, height: 720 },
    },
  },
  webServer: {
    command: 'go run . server --port 18080',
    cwd: cliRoot,
    port: 18080,
    reuseExistingServer: true,
    timeout: 15000,
  },
  outputDir: './demo-output',
})
