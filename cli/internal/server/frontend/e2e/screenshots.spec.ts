import { test, expect } from '@playwright/test'

// DDx Server UI — visual regression screenshots
// These capture each page for visual review and regression detection.
// Run: bunx playwright test e2e/screenshots.spec.ts --update-snapshots
// to update baselines after intentional changes.

test.describe('DDx Server UI Screenshots', () => {
  test('dashboard', async ({ page }) => {
    await page.goto('/')
    await page.waitForSelector('h1')
    // Wait for API data to load
    await page.waitForTimeout(500)
    await expect(page).toHaveScreenshot('dashboard.png', { fullPage: true })
  })

  test('beads kanban board', async ({ page }) => {
    await page.goto('/beads')
    await page.waitForSelector('text=OPEN')
    await page.waitForTimeout(500)
    await expect(page).toHaveScreenshot('beads-kanban.png', { fullPage: true })
  })

  test('documents page', async ({ page }) => {
    await page.goto('/documents')
    await page.waitForTimeout(500)
    await expect(page).toHaveScreenshot('documents.png', { fullPage: true })
  })

  test('graph page', async ({ page }) => {
    await page.goto('/graph')
    await page.waitForTimeout(500)
    await expect(page).toHaveScreenshot('graph.png', { fullPage: true })
  })

  test('agent page', async ({ page }) => {
    await page.goto('/agent')
    await page.waitForTimeout(500)
    await expect(page).toHaveScreenshot('agent.png', { fullPage: true })
  })
})
