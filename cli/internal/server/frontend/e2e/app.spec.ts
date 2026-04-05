import { test, expect } from '@playwright/test'

test.describe('DDx Web UI', () => {
  test('serves the SPA index page', async ({ page }) => {
    await page.goto('/')
    await expect(page).toHaveTitle(/DDx/)
  })

  test('dashboard loads with cards', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('h1')).toContainText('Dashboard')
    // Should have summary cards
    await expect(page.getByRole('heading', { name: 'Documents' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Beads' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Server' })).toBeVisible()
  })

  test('navigates to beads page', async ({ page }) => {
    await page.goto('/')
    await page.click('text=View board')
    await expect(page).toHaveURL(/\/beads/)
    // Kanban columns
    await expect(page.locator('text=OPEN')).toBeVisible()
    await expect(page.locator('text=IN PROGRESS')).toBeVisible()
    await expect(page.locator('text=CLOSED')).toBeVisible()
  })

  test('navigates to documents page', async ({ page }) => {
    await page.goto('/')
    await page.click('a[href="/documents"]')
    await expect(page).toHaveURL(/\/documents/)
  })

  test('navigates to graph page', async ({ page }) => {
    await page.goto('/')
    await page.click('text=View graph')
    await expect(page).toHaveURL(/\/graph/)
  })

  test('health API returns ok', async ({ request }) => {
    const resp = await request.get('/api/health')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(body.status).toBe('ok')
  })

  test('beads API returns array', async ({ request }) => {
    const resp = await request.get('/api/beads')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(Array.isArray(body)).toBeTruthy()
  })
})
