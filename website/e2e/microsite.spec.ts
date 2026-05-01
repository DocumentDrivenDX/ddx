import { test, expect } from '@playwright/test'

test.describe('DDx Microsite', () => {
  test('homepage loads with hero and features', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByText('Documents drive the agents')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Get Started' }).first()).toBeVisible()
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Docs' })).toBeVisible()
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Concepts' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Platform, Not Methodology' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Project-Local by Default' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Validate Your Work' })).toBeVisible()
  })

  test('homepage has all 7 sections', async ({ page }) => {
    await page.goto('/')
    // Section 1: Hero
    await expect(page.locator('.ddx-home-hero')).toBeVisible()
    await expect(page.getByRole('heading', { name: /Documents drive the agents/ })).toBeVisible()
    // Section 2: Problem
    await expect(page.locator('.ddx-home-problem')).toBeVisible()
    await expect(page.getByText('The problem')).toBeVisible()
    // Section 3: How it works
    await expect(page.locator('.ddx-home-how')).toBeVisible()
    await expect(page.getByText('How it works')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Define the work' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Execute in isolation' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Review against criteria' })).toBeVisible()
    // Section 4: Features preview
    await expect(page.locator('.ddx-home-features')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Work Queue' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Execution Engine' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'All features →' })).toBeVisible()
    // Section 5: Principles
    await expect(page.locator('.ddx-home-principles')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Platform, Not Methodology' })).toBeVisible()
    // Section 6: Demo
    await expect(page.locator('.ddx-home-demo')).toBeVisible()
    await expect(page.getByText('See it in action')).toBeVisible()
    // Section 7: CTA
    await expect(page.locator('.ddx-home-cta')).toBeVisible()
    await expect(page.getByRole('heading', { name: /Ready to bring structure/ })).toBeVisible()
  })

  test('homepage screenshot', async ({ page }) => {
    // Block external CDN resources so screenshot is stable across runs
    await page.route('**fonts.googleapis.com**', route => route.abort())
    await page.route('**fonts.gstatic.com**', route => route.abort())
    await page.route('**cdn.jsdelivr.net**', route => route.abort())
    await page.goto('/')
    // Hide async demo player and wait for layout to settle
    await page.addStyleTag({ content: '.ddx-home-demo__player { visibility: hidden !important; }' })
    await page.waitForLoadState('networkidle')
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

  test('features nav link in homepage', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Features' })).toBeVisible()
  })

  test('features page loads with maturity badges', async ({ page }) => {
    await page.goto('/features/')
    await expect(page.getByRole('heading', { name: 'Features' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Bead Tracker' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'MCP Server' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Remote Execution' })).toBeVisible()
    // Maturity badges
    const stableBadges = page.locator('.ddx-maturity--stable')
    await expect(stableBadges.first()).toBeVisible()
    const betaBadges = page.locator('.ddx-maturity--beta')
    await expect(betaBadges.first()).toBeVisible()
    const plannedBadges = page.locator('.ddx-maturity--planned')
    await expect(plannedBadges.first()).toBeVisible()
  })

  test('features page screenshot', async ({ page }) => {
    await page.goto('/features/')
    await page.waitForTimeout(300)
    await expect(page.locator('article, main').first()).toHaveScreenshot('features.png')
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
    await page.goto('/')
    await page.getByRole('navigation').getByText('Features').click()
    await expect(page).toHaveURL(/\/features\//)
  })
})
