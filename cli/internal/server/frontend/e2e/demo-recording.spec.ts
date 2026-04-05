import { test, expect } from '@playwright/test'

// DDx Server UI — Demo Recording
// This test navigates all pages with realistic interactions to produce
// a video recording suitable for embedding in the microsite.
//
// Run with video enabled:
//   npx playwright test e2e/demo-recording.spec.ts --config=playwright.demo.config.ts

test.describe('DDx Server UI Demo', () => {
  test('full walkthrough', async ({ page }) => {
    // Dashboard
    await page.goto('/')
    await page.waitForSelector('h1')
    await page.waitForTimeout(1500)

    // Navigate to Documents
    await page.click('a[href="/documents"]')
    await page.waitForTimeout(1000)
    // Select a document if available
    const docButton = page.locator('.space-y-0\\.5 button').first()
    if (await docButton.isVisible()) {
      await docButton.click()
      await page.waitForTimeout(1500)
    }

    // Navigate to Beads
    await page.click('a[href="/beads"]')
    await page.waitForSelector('text=OPEN')
    await page.waitForTimeout(1000)

    // Use search
    const searchInput = page.locator('input[placeholder*="Search beads"]')
    if (await searchInput.isVisible()) {
      await searchInput.fill('helix')
      await page.waitForTimeout(800)
      await searchInput.fill('')
      await page.waitForTimeout(500)
    }

    // Click a bead card if available
    const beadCard = page.locator('.space-y-2 > [draggable="true"]').first()
    if (await beadCard.isVisible()) {
      await beadCard.click()
      await page.waitForTimeout(1500)
    }

    // Navigate to Graph
    await page.click('a[href="/graph"]')
    await page.waitForTimeout(2000)

    // Navigate to Agent
    await page.click('a[href="/agent"]')
    await page.waitForTimeout(1500)

    // Back to Dashboard
    await page.click('a[href="/"]')
    await page.waitForTimeout(1000)
  })
})
