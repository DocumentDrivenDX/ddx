import { test, expect } from '@playwright/test'

test.describe('DDx Microsite', () => {
  test('homepage loads with hero and features', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByText('Documents drive the agents')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Get Started' })).toBeVisible()
    // Feature cards
    await expect(page.getByRole('heading', { name: 'Work Tracker' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Plugin Registry' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Execution Engine' })).toBeVisible()
  })

  test('homepage screenshot', async ({ page }) => {
    await page.goto('/')
    await page.waitForTimeout(500)
    await expect(page).toHaveScreenshot('homepage.png', { fullPage: true })
  })

  test('getting started page', async ({ page }) => {
    await page.goto('/docs/getting-started/')
    await expect(page.locator('article').getByText('ddx init')).toBeVisible()
    await expect(page.locator('article').getByText('ddx install helix')).toBeVisible()
    await page.waitForTimeout(500)
    await expect(page).toHaveScreenshot('getting-started.png', { fullPage: true })
  })

  test('CLI reference page', async ({ page }) => {
    await page.goto('/docs/cli/')
    await expect(page.getByRole('heading', { name: 'Beads (Work Tracker)' })).toBeVisible()
    await expect(page.getByText('ddx bead create')).toBeVisible()
  })

  test('skills page', async ({ page }) => {
    await page.goto('/docs/skills/')
    await expect(page.getByRole('heading', { name: 'DDx Skills' })).toBeVisible()
    await expect(page.getByRole('cell', { name: '/ddx-bead' })).toBeVisible()
  })

  test('plugins page', async ({ page }) => {
    await page.goto('/docs/plugins/')
    await expect(page.getByRole('heading', { name: 'Creating Plugins' })).toBeVisible()
  })

  test('ecosystem page', async ({ page }) => {
    await page.goto('/docs/ecosystem/')
    await expect(page.getByRole('heading', { name: 'The Stack' })).toBeVisible()
  })

  test('nav links work', async ({ page }) => {
    await page.goto('/')
    await page.getByRole('navigation').getByText('Skills').click()
    await expect(page).toHaveURL(/\/docs\/skills/)
    await page.getByRole('navigation').getByText('Plugins').click()
    await expect(page).toHaveURL(/\/docs\/plugins/)
  })
})
