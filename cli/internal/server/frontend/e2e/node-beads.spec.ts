import { test, expect } from '@playwright/test'

// TC-023: Cross-project beads view at /nodes/:nodeId/beads
//
// Verifies the combined beads table that shows all beads across all projects,
// satisfying the bead ddx-374b9e66 acceptance criteria.

const NODE_ID = 'node-test0001'
const PROJ_A_ID = 'proj-aaaaaaaa'
const PROJ_B_ID = 'proj-bbbbbbbb'

const MOCK_PROJECTS = [
  { id: PROJ_A_ID, name: 'project-alpha', path: '/srv/alpha', registered_at: '2026-01-01T00:00:00Z', last_seen: '2026-01-01T00:00:00Z' },
  { id: PROJ_B_ID, name: 'project-beta', path: '/srv/beta', registered_at: '2026-01-01T00:00:00Z', last_seen: '2026-01-01T00:00:00Z' },
]

const MOCK_BEADS = [
  { id: 'bx-a-001', title: 'Alpha open', status: 'open', priority: 1, issue_type: 'task', project_id: PROJ_A_ID, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'bx-a-002', title: 'Alpha in-progress', status: 'in_progress', priority: 1, issue_type: 'task', project_id: PROJ_A_ID, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'bx-a-003', title: 'Alpha closed', status: 'closed', priority: 2, issue_type: 'task', project_id: PROJ_A_ID, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'bx-b-001', title: 'Beta open', status: 'open', priority: 1, issue_type: 'task', project_id: PROJ_B_ID, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'bx-b-002', title: 'Beta in-progress', status: 'in_progress', priority: 1, issue_type: 'task', project_id: PROJ_B_ID, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'bx-b-003', title: 'Beta closed', status: 'closed', priority: 2, issue_type: 'task', project_id: PROJ_B_ID, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
]

async function stubApis(page: any, beads = MOCK_BEADS) {
  await page.route('**/api/node', async (route: any) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ id: NODE_ID, name: 'test-node', started_at: '2026-01-01T00:00:00Z', last_seen: '2026-01-01T00:00:00Z' }),
    })
  })
  await page.route('**/api/projects', async (route: any) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_PROJECTS),
    })
  })
  await page.route('**/api/beads**', async (route: any) => {
    const url = new URL(route.request().url())
    const statusFilter = url.searchParams.get('status')
    const projectFilter = url.searchParams.get('project_id')
    let filtered = beads
    if (statusFilter) filtered = filtered.filter((b) => b.status === statusFilter)
    if (projectFilter) filtered = filtered.filter((b) => b.project_id === projectFilter)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(filtered),
    })
  })
}

test.describe('TC-023: Cross-project beads view', () => {
  // ---------------------------------------------------------------------------
  // TC-023.1: All six beads appear in the table from two projects.
  // ---------------------------------------------------------------------------
  test('TC-023.1 — table contains beads from at least two distinct projects', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('h1')

    await expect(page.locator('h1')).toContainText('All Beads')
    await expect(page.locator('table')).toBeVisible()

    const rows = page.locator('[data-testid="bead-row"]')
    await expect(rows).toHaveCount(6)
  })

  // ---------------------------------------------------------------------------
  // TC-023.2: Project column is populated for each row.
  // ---------------------------------------------------------------------------
  test('TC-023.2 — Project column shows project names for each row', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('[data-testid="bead-row"]')

    const projectCells = page.locator('[data-testid="project-col"]')
    const count = await projectCells.count()
    expect(count).toBe(6)

    const names = await projectCells.allTextContents()
    expect(names).toContain('project-alpha')
    expect(names).toContain('project-beta')

    for (const name of names) {
      expect(name.trim().length).toBeGreaterThan(0)
    }
  })

  // ---------------------------------------------------------------------------
  // TC-023.3: Status filter chip hides non-matching rows.
  // ---------------------------------------------------------------------------
  test('TC-023.3 — status=open filter chip hides closed and in_progress rows', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('[data-testid="bead-row"]')

    // Click the "open" status chip
    await page.locator('[data-testid="chip-status-open"]').click()

    // Wait for the table to update
    await expect(page.locator('[data-testid="bead-row"]')).toHaveCount(2, { timeout: 5000 })

    // All visible rows should be open
    const rows = page.locator('[data-testid="bead-row"]')
    for (let i = 0; i < await rows.count(); i++) {
      await expect(rows.nth(i).locator('span')).toContainText('open')
    }
  })

  // ---------------------------------------------------------------------------
  // TC-023.4: URL query params reflect active filters.
  // ---------------------------------------------------------------------------
  test('TC-023.4 — active filters are reflected in URL query params', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('[data-testid="bead-row"]')

    await page.locator('[data-testid="chip-status-open"]').click()
    await expect(page).toHaveURL(new RegExp(`status=open`))
  })

  // ---------------------------------------------------------------------------
  // TC-023.5: Project filter chip narrows table to one project's beads.
  // ---------------------------------------------------------------------------
  test('TC-023.5 — project filter chip narrows table to one project', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('[data-testid="bead-row"]')

    // Click project-alpha chip
    await page.locator(`[data-testid="chip-project-${PROJ_A_ID}"]`).click()

    // Should show only project-alpha's 3 beads
    await expect(page.locator('[data-testid="bead-row"]')).toHaveCount(3, { timeout: 5000 })

    // All visible rows should show project-alpha
    const projectCells = page.locator('[data-testid="project-col"]')
    const names = await projectCells.allTextContents()
    for (const name of names) {
      expect(name).toBe('project-alpha')
    }
  })

  // ---------------------------------------------------------------------------
  // TC-023.6: Project filter is encoded in URL query params.
  // ---------------------------------------------------------------------------
  test('TC-023.6 — project filter is reflected in URL query params', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('[data-testid="bead-row"]')

    await page.locator(`[data-testid="chip-project-${PROJ_A_ID}"]`).click()
    await expect(page).toHaveURL(new RegExp(`project_id=${PROJ_A_ID}`))
  })

  // ---------------------------------------------------------------------------
  // TC-023.7: Auto-refresh indicator is visible.
  // ---------------------------------------------------------------------------
  test('TC-023.7 — auto-refresh 10s indicator is visible', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('h1')

    await expect(page.locator('text=auto-refresh 10s')).toBeVisible()
  })

  // ---------------------------------------------------------------------------
  // TC-023.8: Table headers include all required columns.
  // ---------------------------------------------------------------------------
  test('TC-023.8 — table shows all required column headers', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('table')

    await expect(page.locator('th:has-text("Project")')).toBeVisible()
    await expect(page.locator('th:has-text("ID")')).toBeVisible()
    await expect(page.locator('th:has-text("Title")')).toBeVisible()
    await expect(page.locator('th:has-text("Status")')).toBeVisible()
  })

  // ---------------------------------------------------------------------------
  // TC-023.9: Empty state is shown when no beads match filters.
  // ---------------------------------------------------------------------------
  test('TC-023.9 — shows empty state when no beads match filters', async ({ page }) => {
    await stubApis(page, [])
    await page.goto(`/nodes/${NODE_ID}/beads`)
    await page.waitForSelector('h1')

    await expect(page.locator('[data-testid="empty-state"]')).toBeVisible()
    await expect(page.locator('[data-testid="bead-row"]')).toHaveCount(0)
  })
})
