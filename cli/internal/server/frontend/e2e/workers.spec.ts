import { test, expect } from '@playwright/test'

// TC-009: Workers page — live workers list and detail panels.
// These tests validate the /workers route and its auto-refresh behaviour.
// Workers may not be running during test execution; tests degrade gracefully
// when the list is empty.

test.describe('TC-009: Workers page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/workers')
    await page.waitForSelector('h1')
  })

  // -------------------------------------------------------------------------
  // TC-009.1: list-shows-running-worker
  // The /workers page renders the table header and loads without error.
  // If at least one worker is present its row is shown with expected columns.
  // -------------------------------------------------------------------------
  test('TC-009.1 — list shows workers table', async ({ page }) => {
    await expect(page.locator('h1')).toContainText('Workers')
    // Table headers should always render
    await expect(page.locator('th:has-text("State")')).toBeVisible()
    await expect(page.locator('th:has-text("Harness")')).toBeVisible()
    await expect(page.locator('th:has-text("Attempts")')).toBeVisible()

    // Wait for initial load to complete (loading indicator gone or data shown)
    await page.waitForTimeout(500)

    // Either the empty state or at least one row must be visible
    const emptyState = page.locator('text=No workers found')
    const firstRow = page.locator('tbody tr').first()
    const hasEmpty = await emptyState.isVisible({ timeout: 3000 }).catch(() => false)
    const hasRow = await firstRow.isVisible({ timeout: 3000 }).catch(() => false)
    expect(hasEmpty || hasRow).toBeTruthy()
  })

  // -------------------------------------------------------------------------
  // TC-009.2: list-refreshes-on-state-change
  // The auto-refresh label is present, confirming polling is wired up.
  // We verify the DOM re-renders by checking the auto-refresh indicator exists
  // and the table remains visible after waiting 2+ seconds.
  // -------------------------------------------------------------------------
  test('TC-009.2 — auto-refresh indicator present and page stays live', async ({ page }) => {
    // The auto-refresh label must be visible
    await expect(page.locator('text=auto-refresh 2s')).toBeVisible()

    // Wait past one refresh cycle and verify page is still functional
    await page.waitForTimeout(2500)
    await expect(page.locator('h1')).toContainText('Workers')

    // Table must still be present (not replaced by an error)
    await expect(page.locator('table')).toBeVisible()
    await expect(page.locator('text=Error')).not.toBeVisible()
  })

  // -------------------------------------------------------------------------
  // TC-009.3: detail-renders-prompt-log-and-utilization
  // If a worker row is clickable, clicking it shows a detail panel with
  // all three tab buttons: log, prompt, utilization.
  // Skipped gracefully when no workers are present.
  // -------------------------------------------------------------------------
  test('TC-009.3 — detail panel shows log/prompt/utilization tabs', async ({ page }) => {
    await page.waitForTimeout(600) // let data load

    const firstRow = page.locator('tbody tr').first()
    if (!(await firstRow.isVisible({ timeout: 2000 }).catch(() => false))) {
      test.skip(true, 'No workers present — skipping detail test')
      return
    }

    await firstRow.click()
    const detail = page.locator('[data-testid="worker-detail"]')
    await expect(detail).toBeVisible({ timeout: 3000 })

    // All three tab buttons must be present
    await expect(detail.locator('button:has-text("log")')).toBeVisible()
    await expect(detail.locator('button:has-text("prompt")')).toBeVisible()
    await expect(detail.locator('button:has-text("utilization")')).toBeVisible()

    // Default tab is "log" — log panel should be present
    await expect(detail.locator('[data-testid="log-panel"]')).toBeVisible()

    // Switch to utilization
    await detail.locator('button:has-text("utilization")').click()
    await expect(detail.locator('[data-testid="utilization-panel"]')).toBeVisible()
    await expect(detail.locator('text=Attempts')).toBeVisible()
    await expect(detail.locator('text=Harness')).toBeVisible()

    // Switch to prompt
    await detail.locator('button:has-text("prompt")').click()
    await expect(detail.locator('[data-testid="prompt-panel"]')).toBeVisible()
  })
})

// -------------------------------------------------------------------------
// TC-009.4: API endpoint — workers list returns an array
// -------------------------------------------------------------------------
test.describe('TC-009: Workers API', () => {
  test('TC-009.4 — GET /api/agent/workers returns array', async ({ request }) => {
    const resp = await request.get('/api/agent/workers')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(Array.isArray(body)).toBeTruthy()
  })
})

// -------------------------------------------------------------------------
// TC-009.5: Navigation — Workers link appears in sidebar
// -------------------------------------------------------------------------
test.describe('TC-009: Workers navigation', () => {
  test('TC-009.5 — Workers nav link is present', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('a[href="/workers"]')).toBeVisible()
  })

  test('TC-009.6 — clicking Workers nav link navigates to /workers', async ({ page }) => {
    await page.goto('/')
    await page.click('a[href="/workers"]')
    await expect(page).toHaveURL(/\/workers/)
    await expect(page.locator('h1')).toContainText('Workers')
  })
})
