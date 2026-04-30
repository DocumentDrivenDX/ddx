import { test, expect } from '@playwright/test'

test.describe('DDx Microsite', () => {
  test('homepage loads with hero and features', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByText('Documents drive the agents')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Get Started' })).toBeVisible()
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Docs' })).toBeVisible()
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Concepts' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Platform, Not Methodology' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Project-Local by Default' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Validate Your Work' })).toBeVisible()
  })

  test('homepage screenshot', async ({ page }) => {
    await page.goto('/')
    await page.waitForTimeout(500)
    await expect(page).toHaveScreenshot('homepage.png', { fullPage: true })
  })

  test('homepage fits mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/')
    await expect(page.getByRole('heading', { name: /Documents drive the agents/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Platform, Not Methodology' })).toBeVisible()
    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth)
    expect(scrollWidth).toBeLessThanOrEqual(390)
  })

  test('getting started page', async ({ page }) => {
    await page.goto('/docs/getting-started/')
    await expect(page.locator('article').getByText('ddx init').first()).toBeVisible()
    await expect(page.locator('article').getByText('ddx install helix').first()).toBeVisible()
    await page.addStyleTag({ content: '.asciinema-container { display: none !important; }' })
    await page.waitForTimeout(500)
    await expect(page.locator('article')).toHaveScreenshot('getting-started.png')
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
    await expect(page.getByRole('heading', { name: 'Plugins' })).toBeVisible()
  })

  test('ecosystem page', async ({ page }) => {
    await page.goto('/docs/ecosystem/')
    await expect(page.getByRole('heading', { name: 'The Stack' })).toBeVisible()
  })

  test('nav links work', async ({ page }) => {
    await page.goto('/')
    await page.getByRole('navigation').getByText('Docs').click()
    await expect(page).toHaveURL(/\/docs\//)
    await page.goto('/')
    await page.getByRole('navigation').getByText('Concepts').click()
    await expect(page).toHaveURL(/\/docs\/concepts/)
  })
})
