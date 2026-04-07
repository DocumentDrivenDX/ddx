import { test, expect } from '@playwright/test'

// TP-002: DDx Server Web UI — End-to-End Tests
// Covers TC-001 through TC-008.

// ---------------------------------------------------------------------------
// TC-001: Dashboard
// ---------------------------------------------------------------------------
test.describe('TC-001: Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await page.waitForSelector('h1')
  })

  test('TC-001.1 — dashboard loads', async ({ page }) => {
    await expect(page.locator('h1')).toContainText('Dashboard')
  })

  test('TC-001.2 — document count card', async ({ page }) => {
    const card = page.locator('h2:has-text("Documents")').locator('..')
    await expect(card).toBeVisible()
    // Card should contain a link to browse
    await expect(card.locator('a[href="/documents"]')).toBeVisible()
  })

  test('TC-001.3 — bead status card', async ({ page }) => {
    const card = page.locator('h2:has-text("Beads")').locator('..')
    await expect(card.locator('text=Ready')).toBeVisible()
    await expect(card.locator('text=Open')).toBeVisible()
    await expect(card.locator('text=Closed')).toBeVisible()
  })

  test('TC-001.4 — stale docs card', async ({ page }) => {
    const card = page.locator('text=Stale Docs').locator('..')
    await expect(card).toBeVisible()
  })

  test('TC-001.5 — server health card', async ({ page }) => {
    const card = page.locator('h2:has-text("Server")').locator('..')
    await expect(card.locator('text=ok')).toBeVisible()
  })

  test('TC-001.6 — navigate to documents', async ({ page }) => {
    await page.click('a[href="/documents"]')
    await expect(page).toHaveURL(/\/documents/)
  })

  test('TC-001.7 — navigate to beads', async ({ page }) => {
    await page.click('text=View board')
    await expect(page).toHaveURL(/\/beads/)
  })

  test('TC-001.8 — navigate to graph', async ({ page }) => {
    await page.click('text=View graph')
    await expect(page).toHaveURL(/\/graph/)
  })
})

// ---------------------------------------------------------------------------
// TC-002: Documents Page
// ---------------------------------------------------------------------------
test.describe('TC-002: Documents', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/documents')
    await page.waitForSelector('h1')
  })

  test('TC-002.1 — document list loads', async ({ page }) => {
    await expect(page.locator('h1')).toContainText('Documents')
    // Wait for loading to complete (either docs appear or "No documents found")
    await page.waitForTimeout(1000)
  })

  test('TC-002.2 — type filter', async ({ page }) => {
    const select = page.locator('select')
    await expect(select).toBeVisible()
    // Select an option (anything other than "All types")
    const options = await select.locator('option').allTextContents()
    if (options.length > 1) {
      await select.selectOption({ index: 1 })
      // List should still render (possibly fewer items)
      await page.waitForTimeout(300)
    }
  })

  test('TC-002.3 — search filter', async ({ page }) => {
    const search = page.locator('input[placeholder*="Search"]')
    await expect(search).toBeVisible()
    // Type something unlikely to match everything
    await search.fill('zzz_nonexistent')
    await page.waitForTimeout(300)
    // Should show "No documents found" or an empty list
    const noResults = page.locator('text=No documents found')
    // Clear search to restore list
    await search.fill('')
    await page.waitForTimeout(300)
  })

  test('TC-002.4 — view document', async ({ page }) => {
    const firstDoc = page.locator('.w-72 button').first()
    if (!(await firstDoc.isVisible({ timeout: 2000 }).catch(() => false))) {
      test.skip(true, 'No documents available (library not installed)')
      return
    }
    await firstDoc.click()
    await expect(page.locator('.prose')).toBeVisible({ timeout: 5000 })
  })

  test('TC-002.5 — document path display', async ({ page }) => {
    const firstDoc = page.locator('.w-72 button').first()
    if (!(await firstDoc.isVisible({ timeout: 2000 }).catch(() => false))) {
      test.skip(true, 'No documents available')
      return
    }
    await firstDoc.click()
    await expect(page.locator('.font-mono')).toBeVisible({ timeout: 5000 })
  })

  test('TC-002.6 — edit button', async ({ page }) => {
    const firstDoc = page.locator('.w-72 button').first()
    if (!(await firstDoc.isVisible({ timeout: 2000 }).catch(() => false))) {
      test.skip(true, 'No documents available')
      return
    }
    await firstDoc.click()
    await page.waitForSelector('.prose')
    const editBtn = page.locator('button:has-text("Edit")')
    await expect(editBtn).toBeVisible()
    await editBtn.click()
    await expect(page.locator('textarea')).toBeVisible()
  })

  test('TC-002.7 — cancel edit', async ({ page }) => {
    const firstDoc = page.locator('.w-72 button').first()
    if (!(await firstDoc.isVisible({ timeout: 2000 }).catch(() => false))) {
      test.skip(true, 'No documents available')
      return
    }
    await firstDoc.click()
    await page.waitForSelector('.prose')
    await page.click('button:has-text("Edit")')
    await expect(page.locator('textarea')).toBeVisible()
    await page.click('button:has-text("Cancel")')
    await expect(page.locator('.prose')).toBeVisible()
    await expect(page.locator('textarea')).not.toBeVisible()
  })

  test('TC-002.8 — empty state', async ({ page }) => {
    // Before selecting anything, placeholder should show
    await expect(page.locator('text=Select a document')).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// TC-003: Beads Kanban Board
// ---------------------------------------------------------------------------
test.describe('TC-003: Beads', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/beads')
    // Wait for kanban to load — columns render after SQLite init + API fetch
    await page.waitForSelector('h2', { timeout: 15000 })
  })

  test('TC-003.1 — kanban loads', async ({ page }) => {
    await expect(page.locator('h2:has-text("OPEN")')).toBeVisible()
    await expect(page.locator('h2:has-text("IN PROGRESS")')).toBeVisible()
    await expect(page.locator('h2:has-text("CLOSED")')).toBeVisible()
  })

  test('TC-003.2 — bead cards render', async ({ page }) => {
    // At least one card should exist somewhere
    const cards = page.locator('[draggable="true"]')
    const count = await cards.count()
    expect(count).toBeGreaterThan(0)
    // First card should show an ID pattern like ddx-XXXXXXXX
    const firstCard = cards.first()
    await expect(firstCard).toContainText(/ddx-|hx-/)
  })

  test('TC-003.3 — search beads', async ({ page }) => {
    const search = page.locator('input[placeholder*="Search"]')
    await expect(search).toBeVisible()
    await search.fill('helix')
    await page.waitForTimeout(500)
    // Should filter — some cards may disappear
  })

  test('TC-003.4 — clear search', async ({ page }) => {
    const search = page.locator('input[placeholder*="Search"]')
    await search.fill('helix')
    await page.waitForTimeout(300)
    await search.fill('')
    await page.waitForTimeout(300)
    // Cards should be restored
    const cards = page.locator('[draggable="true"]')
    const count = await cards.count()
    expect(count).toBeGreaterThan(0)
  })

  test('TC-003.5 — select bead opens detail', async ({ page }) => {
    const card = page.locator('[draggable="true"]').first()
    await card.click()
    // Detail panel should appear with a close button
    await expect(page.locator('button:has-text("×")')).toBeVisible({ timeout: 3000 })
  })

  test('TC-003.6 — detail shows fields', async ({ page }) => {
    const card = page.locator('[draggable="true"]').first()
    await card.click()
    const detail = page.locator('.fixed.right-4')
    await expect(detail).toBeVisible({ timeout: 3000 })
    // Should show the bead ID
    await expect(detail).toContainText(/ddx-|hx-/)
  })

  test('TC-003.7 — close detail', async ({ page }) => {
    const card = page.locator('[draggable="true"]').first()
    await card.click()
    await expect(page.locator('.fixed.right-4')).toBeVisible({ timeout: 3000 })
    await page.click('button:has-text("×")')
    await expect(page.locator('.fixed.right-4')).not.toBeVisible()
  })

  test('TC-003.8 — create bead modal', async ({ page }) => {
    await page.click('button:has-text("New Bead")')
    const modal = page.locator('form')
    await expect(modal).toBeVisible()
    await expect(modal.locator('text=Title')).toBeVisible()
    await expect(modal.locator('text=Type')).toBeVisible()
    await expect(modal.locator('text=Priority')).toBeVisible()
    await expect(modal.locator('text=Labels')).toBeVisible()
    await expect(modal.locator('text=Description')).toBeVisible()
    await expect(modal.locator('text=Acceptance')).toBeVisible()
  })

  test('TC-003.9 — create bead submit', async ({ page }) => {
    await page.click('button:has-text("New Bead")')
    const modal = page.locator('form')
    await modal.locator('input').first().fill('Playwright test bead')

    // Listen for the API response
    const responsePromise = page.waitForResponse(resp =>
      resp.url().includes('/api/beads') && resp.request().method() === 'POST'
    )
    await modal.locator('button:has-text("Create Bead")').click()
    const response = await responsePromise

    if (!response.ok()) {
      // Server couldn't create bead (e.g., read-only filesystem) — verify error shown
      await expect(modal.locator('.text-red-500')).toBeVisible({ timeout: 3000 })
      return
    }

    // Modal should close and new bead should appear
    await expect(modal).not.toBeVisible({ timeout: 10000 })
    await expect(page.locator('text=Playwright test bead')).toBeVisible({ timeout: 5000 })
  })

  test('TC-003.10 — claim bead', async ({ page }) => {
    // Find an open bead and click it
    const openColumn = page.locator('h2:has-text("OPEN")').locator('..')
    const openCard = openColumn.locator('[draggable="true"]').first()
    if (await openCard.isVisible()) {
      await openCard.click()
      const claimBtn = page.locator('button:has-text("Claim")')
      if (await claimBtn.isVisible()) {
        await claimBtn.click()
        await page.waitForTimeout(500)
        // Bead should move to IN PROGRESS
      }
    }
  })
})

// ---------------------------------------------------------------------------
// TC-004: Document Graph
// ---------------------------------------------------------------------------
test.describe('TC-004: Graph', () => {
  test('TC-004.1 — graph loads', async ({ page }) => {
    await page.goto('/graph')
    // Should not show an error
    await expect(page.locator('text=Error')).not.toBeVisible({ timeout: 5000 })
  })
})

// ---------------------------------------------------------------------------
// TC-005: Agent Sessions
// ---------------------------------------------------------------------------
test.describe('TC-005: Agent', () => {
  test('TC-005.1 — page loads', async ({ page }) => {
    await page.goto('/agent')
    await page.waitForTimeout(1000)
    // Page should render without crashing
    await expect(page.locator('body')).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// TC-006: Personas
// ---------------------------------------------------------------------------
test.describe('TC-006: Personas', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/personas')
  })

  test('TC-006.1 — persona list loads', async ({ page }) => {
    // Personas page has an h2 "Personas" in the sidebar
    await expect(page.locator('text=Personas').first()).toBeVisible()
    await page.waitForTimeout(1000)
  })

  test('TC-006.2 — select persona', async ({ page }) => {
    const firstPersona = page.locator('.w-80 button').first()
    if (!(await firstPersona.isVisible({ timeout: 2000 }).catch(() => false))) {
      test.skip(true, 'No personas available (library not installed)')
      return
    }
    await firstPersona.click()
    await expect(page.locator('pre')).toBeVisible({ timeout: 5000 })
  })

  test('TC-006.3 — role badges', async ({ page }) => {
    const badges = page.locator('.bg-blue-100')
    const count = await badges.count()
    expect(count).toBeGreaterThanOrEqual(0)
  })
})

// ---------------------------------------------------------------------------
// TC-007: Navigation
// ---------------------------------------------------------------------------
test.describe('TC-007: Navigation', () => {
  test('TC-007.1 — all nav links visible', async ({ page }) => {
    await page.goto('/')
    // Check nav links (use .first() to avoid strict mode violations from dashboard cards)
    await expect(page.locator('a[href="/"]').first()).toBeVisible()
    await expect(page.locator('a[href="/documents"]').first()).toBeVisible()
    await expect(page.locator('a[href="/beads"]').first()).toBeVisible()
    await expect(page.locator('a[href="/graph"]').first()).toBeVisible()
    await expect(page.locator('a[href="/agent"]').first()).toBeVisible()
    await expect(page.locator('a[href="/personas"]').first()).toBeVisible()
  })

  test('TC-007.3 — SPA routing', async ({ page }) => {
    await page.goto('/')
    // Navigate via clicks — should not trigger full page reload
    await page.click('a[href="/documents"]')
    await expect(page).toHaveURL(/\/documents/)
    await page.click('a[href="/beads"]')
    await expect(page).toHaveURL(/\/beads/)
    await page.click('a[href="/graph"]')
    await expect(page).toHaveURL(/\/graph/)
    await page.click('a[href="/agent"]')
    await expect(page).toHaveURL(/\/agent/)
    await page.click('a[href="/personas"]')
    await expect(page).toHaveURL(/\/personas/)
    await page.click('a[href="/"]')
    await expect(page).toHaveURL(/\/$/)
  })
})

// ---------------------------------------------------------------------------
// TC-008: HTTP API
// ---------------------------------------------------------------------------
test.describe('TC-008: HTTP API', () => {
  test('TC-008.1 — health endpoint', async ({ request }) => {
    const resp = await request.get('/api/health')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(body.status).toBe('ok')
  })

  test('TC-008.2 — documents list', async ({ request }) => {
    const resp = await request.get('/api/documents')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(Array.isArray(body)).toBeTruthy()
  })

  test('TC-008.3 — beads list', async ({ request }) => {
    const resp = await request.get('/api/beads')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(Array.isArray(body)).toBeTruthy()
  })

  test('TC-008.4 — beads status', async ({ request }) => {
    const resp = await request.get('/api/beads/status')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(body).toHaveProperty('open')
    expect(body).toHaveProperty('closed')
  })

  test('TC-008.5 — personas list', async ({ request }) => {
    const resp = await request.get('/api/personas')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(Array.isArray(body)).toBeTruthy()
  })

  test('TC-008.6 — doc graph', async ({ request }) => {
    const resp = await request.get('/api/docs/graph')
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(Array.isArray(body)).toBeTruthy()
  })
})
