import { test, expect } from '@playwright/test'

// TC-022: Cross-project workers view at /nodes/:nodeId/agents
//
// Verifies the combined workers table that shows all running workers across all
// projects on a single screen, satisfying the bead ddx-f2e9bdee acceptance criteria.

const NODE_ID = 'node-test0001'
const PROJ_A_ID = 'proj-aaaaaaaa'
const PROJ_B_ID = 'proj-bbbbbbbb'
const PROJ_C_ID = 'proj-cccccccc'

const MOCK_PROJECTS = [
  { id: PROJ_A_ID, name: 'project-alpha', path: '/srv/alpha', registered_at: '2026-01-01T00:00:00Z', last_seen: '2026-01-01T00:00:00Z' },
  { id: PROJ_B_ID, name: 'project-beta', path: '/srv/beta', registered_at: '2026-01-01T00:00:00Z', last_seen: '2026-01-01T00:00:00Z' },
  { id: PROJ_C_ID, name: 'project-gamma', path: '/srv/gamma', registered_at: '2026-01-01T00:00:00Z', last_seen: '2026-01-01T00:00:00Z' },
]

const MOCK_WORKERS = [
  {
    id: 'worker-alpha-001',
    kind: 'execute-loop',
    state: 'running',
    project_root: '/srv/alpha',
    harness: 'claude',
    model: 'claude-sonnet-4-6',
    started_at: '2026-01-01T12:00:00Z',
    attempts: 2,
    last_result: { status: 'ok' },
  },
  {
    id: 'worker-beta-0022',
    kind: 'execute-loop',
    state: 'exited',
    project_root: '/srv/beta',
    harness: 'codex',
    model: 'gpt-5',
    started_at: '2026-01-01T11:00:00Z',
    attempts: 1,
    last_result: { status: 'no_changes' },
  },
  {
    id: 'worker-gamma-003',
    kind: 'execute-loop',
    state: 'running',
    project_root: '/srv/gamma',
    harness: 'claude',
    model: 'claude-opus-4-6',
    started_at: '2026-01-01T10:00:00Z',
    attempts: 3,
    last_result: { status: 'merged' },
  },
]

// Stub all three API endpoints used by the NodeAgents page.
async function stubApis(page: any, workers = MOCK_WORKERS) {
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
  await page.route('**/api/agent/workers', async (route: any) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(workers),
    })
  })
}

test.describe('TC-022: Cross-project workers view', () => {
  // ---------------------------------------------------------------------------
  // TC-022.1: All three workers appear in the table.
  // ---------------------------------------------------------------------------
  test('TC-022.1 — all workers from all projects appear in the table', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/agents`)
    await page.waitForSelector('h1')

    await expect(page.locator('h1')).toContainText('All Agents')
    await expect(page.locator('table')).toBeVisible()

    // All three rows must be present.
    const rows = page.locator('tbody tr')
    await expect(rows).toHaveCount(3)
  })

  // ---------------------------------------------------------------------------
  // TC-022.2: Each row has a non-empty Project column matching the seeded project name.
  // ---------------------------------------------------------------------------
  test('TC-022.2 — each row shows a non-empty project name', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/agents`)
    await page.waitForSelector('tbody tr')

    const projectCells = page.locator('[data-testid="project-col"]')
    const count = await projectCells.count()
    expect(count).toBe(3)

    const names = await projectCells.allTextContents()
    expect(names).toContain('project-alpha')
    expect(names).toContain('project-beta')
    expect(names).toContain('project-gamma')

    // No cell should be empty.
    for (const name of names) {
      expect(name.trim().length).toBeGreaterThan(0)
    }
  })

  // ---------------------------------------------------------------------------
  // TC-022.3: Row order is by started-at descending (server already sorts; UI preserves it).
  // ---------------------------------------------------------------------------
  test('TC-022.3 — rows are ordered by started-at descending', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/agents`)
    await page.waitForSelector('tbody tr')

    const rows = page.locator('tbody tr')
    // MOCK_WORKERS is already sorted descending (12:00, 11:00, 10:00).
    // The first row must show 'worker-alpha-001' (latest started_at).
    const firstRowProjectCell = rows.nth(0).locator('[data-testid="project-col"]')
    await expect(firstRowProjectCell).toHaveText('project-alpha')

    const lastRowProjectCell = rows.nth(2).locator('[data-testid="project-col"]')
    await expect(lastRowProjectCell).toHaveText('project-gamma')
  })

  // ---------------------------------------------------------------------------
  // TC-022.4: Clicking a row navigates to /nodes/:nodeId/projects/:projectId/agents/:workerId.
  // ---------------------------------------------------------------------------
  test('TC-022.4 — clicking a row navigates to the correct per-worker detail URL', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/agents`)
    await page.waitForSelector('tbody tr')

    // Click the first row (project-alpha worker).
    await page.locator('tbody tr').first().click()

    await expect(page).toHaveURL(
      new RegExp(`/nodes/${NODE_ID}/projects/${PROJ_A_ID}/agents/worker-alpha-001`),
    )
  })

  // ---------------------------------------------------------------------------
  // TC-022.5: When a worker's state transitions from running to exited, the
  //           table updates within one refresh cycle (~3 seconds).
  // ---------------------------------------------------------------------------
  test('TC-022.5 — table updates when worker state transitions running→exited', async ({ page }) => {
    // First response: worker-alpha-001 is running.
    let callCount = 0
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
    await page.route('**/api/agent/workers', async (route: any) => {
      callCount++
      const workers = callCount === 1
        ? MOCK_WORKERS
        : MOCK_WORKERS.map((w) =>
            w.id === 'worker-alpha-001' ? { ...w, state: 'exited' } : w,
          )
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(workers),
      })
    })

    await page.goto(`/nodes/${NODE_ID}/agents`)
    await page.waitForSelector('tbody tr')

    // First render: worker-alpha-001 is running.
    const firstRow = page.locator('tbody tr').first()
    await expect(firstRow.locator('span:has-text("running")')).toBeVisible()

    // Wait for the second refresh cycle (3s + margin).
    await page.waitForTimeout(4000)

    // After refresh: worker-alpha-001 should now show exited.
    await expect(firstRow.locator('span:has-text("exited")')).toBeVisible({ timeout: 5000 })
  })

  // ---------------------------------------------------------------------------
  // TC-022.6: Empty-state message when no workers are running.
  // ---------------------------------------------------------------------------
  test('TC-022.6 — shows empty-state message when no workers are present', async ({ page }) => {
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
    await page.route('**/api/agent/workers', async (route: any) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      })
    })

    await page.goto(`/nodes/${NODE_ID}/agents`)
    await page.waitForSelector('h1')

    await expect(page.locator('[data-testid="empty-state"]')).toBeVisible()
    await expect(page.locator('tbody tr')).toHaveCount(0)
  })

  // ---------------------------------------------------------------------------
  // TC-022.7: Auto-refresh indicator is present.
  // ---------------------------------------------------------------------------
  test('TC-022.7 — auto-refresh 3s indicator is visible', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/agents`)
    await page.waitForSelector('h1')

    await expect(page.locator('text=auto-refresh 3s')).toBeVisible()
  })

  // ---------------------------------------------------------------------------
  // TC-022.8: Project picker "All projects" navigates to /nodes/:nodeId/agents.
  // ---------------------------------------------------------------------------
  test('TC-022.8 — project picker "All projects" navigates to the combined view', async ({ page }) => {
    await stubApis(page)
    await page.goto('/')
    await page.waitForSelector('nav')

    // Wait for the project picker to appear (requires node + projects to load).
    const picker = page.locator('[data-testid="project-picker"]')
    await expect(picker).toBeVisible({ timeout: 5000 })

    await picker.selectOption('__all__')
    await expect(page).toHaveURL(new RegExp(`/nodes/${NODE_ID}/agents`))
  })

  // ---------------------------------------------------------------------------
  // TC-022.9: Table headers include all required columns.
  // ---------------------------------------------------------------------------
  test('TC-022.9 — table shows all required column headers', async ({ page }) => {
    await stubApis(page)
    await page.goto(`/nodes/${NODE_ID}/agents`)
    await page.waitForSelector('table')

    await expect(page.locator('th:has-text("Project")')).toBeVisible()
    await expect(page.locator('th:has-text("Worker ID")')).toBeVisible()
    await expect(page.locator('th:has-text("Bead")')).toBeVisible()
    await expect(page.locator('th:has-text("Harness")')).toBeVisible()
    await expect(page.locator('th:has-text("Model")')).toBeVisible()
    await expect(page.locator('th:has-text("State")')).toBeVisible()
    await expect(page.locator('th:has-text("Started At")')).toBeVisible()
    await expect(page.locator('th:has-text("Attempts")')).toBeVisible()
    await expect(page.locator('th:has-text("Last Status")')).toBeVisible()
  })
})
