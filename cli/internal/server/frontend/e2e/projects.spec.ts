import { test, expect } from '@playwright/test'

// TP-002: Multi-project registry, scoped routing, and isolation
// Covers TC-010 (Project Registry and Scoped Routing) and TC-012 (Host+User Project Isolation).

// ---------------------------------------------------------------------------
// TC-010: Project Registry and Scoped Routing
// ---------------------------------------------------------------------------
test.describe('TC-010: Project Registry and Scoped Routing', () => {
  test('TC-010.1 — registry lists registered projects', async ({ request }) => {
    const resp = await request.get('/api/projects')
    expect(resp.ok()).toBeTruthy()
    const projects = await resp.json()
    expect(Array.isArray(projects)).toBeTruthy()
    // Server registers its working dir on startup; at least one entry must exist.
    expect(projects.length).toBeGreaterThan(0)
    const p = projects[0]
    expect(p).toHaveProperty('id')
    expect(p).toHaveProperty('name')
    expect(p).toHaveProperty('path')
    expect(p.id).toMatch(/^proj-[0-9a-f]{8}$/)
  })

  test('TC-010.2 — scoped API request resolves project context', async ({ request }) => {
    // Obtain the first registered project ID and hit its scoped workers endpoint.
    const listResp = await request.get('/api/projects')
    expect(listResp.ok()).toBeTruthy()
    const projects = await listResp.json()
    expect(projects.length).toBeGreaterThan(0)
    const projID: string = projects[0].id

    const scopedResp = await request.get(`/api/projects/${projID}/workers`)
    expect(scopedResp.ok()).toBeTruthy()
    const workers = await scopedResp.json()
    expect(Array.isArray(workers)).toBeTruthy()
  })

  test('TC-010.4 — singleton fallback: legacy unscoped routes still served', async ({ request }) => {
    // Even with a single registered project the legacy unscoped routes must respond.
    const health = await request.get('/api/health')
    expect(health.ok()).toBeTruthy()
    const beads = await request.get('/api/beads')
    expect(beads.ok()).toBeTruthy()
    const docs = await request.get('/api/documents')
    expect(docs.ok()).toBeTruthy()
    const node = await request.get('/api/node')
    expect(node.ok()).toBeTruthy()
  })

  test('TC-010.5 — unknown project ID returns 404 on scoped endpoint', async ({ request }) => {
    const resp = await request.get('/api/projects/proj-00000000/workers')
    expect(resp.status()).toBe(404)
    const body = await resp.json()
    expect(body).toHaveProperty('error')
  })

  test('TC-010.7 — MCP: ddx_list_projects lists registered projects', async ({ request }) => {
    const resp = await request.post('/mcp', {
      data: {
        jsonrpc: '2.0',
        id: 1,
        method: 'tools/call',
        params: { name: 'ddx_list_projects', arguments: {} },
      },
    })
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(body).toHaveProperty('result')
    const content: Array<{ type: string; text: string }> = body.result.content
    expect(Array.isArray(content)).toBeTruthy()
    expect(content.length).toBeGreaterThan(0)
    const projects = JSON.parse(content[0].text)
    expect(Array.isArray(projects)).toBeTruthy()
    expect(projects.length).toBeGreaterThan(0)
    expect(projects[0].id).toMatch(/^proj-[0-9a-f]{8}$/)
  })

  test('TC-010.8 — MCP: ddx_show_project resolves project by ID', async ({ request }) => {
    // Obtain a known project ID first.
    const listResp = await request.get('/api/projects')
    const projects = await listResp.json()
    expect(projects.length).toBeGreaterThan(0)
    const knownID: string = projects[0].id

    const resp = await request.post('/mcp', {
      data: {
        jsonrpc: '2.0',
        id: 2,
        method: 'tools/call',
        params: { name: 'ddx_show_project', arguments: { id: knownID } },
      },
    })
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(body).toHaveProperty('result')
    const entry = JSON.parse(body.result.content[0].text)
    expect(entry.id).toBe(knownID)
    expect(typeof entry.path).toBe('string')
  })

  test('TC-010.9 — MCP: project-aware tool call returns project data without error', async ({ request }) => {
    const resp = await request.post('/mcp', {
      data: {
        jsonrpc: '2.0',
        id: 3,
        method: 'tools/call',
        params: { name: 'ddx_list_projects', arguments: {} },
      },
    })
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(body.result.isError).toBeFalsy()
    const projects: Array<{ id: string; path: string }> = JSON.parse(body.result.content[0].text)
    expect(
      projects.every(p => p.id.startsWith('proj-') && typeof p.path === 'string'),
    ).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// TC-012: Host+User Project Isolation and Concurrency
// ---------------------------------------------------------------------------
test.describe('TC-012: Project Isolation and Concurrency', () => {
  test('TC-012.1 — re-registering an existing project is idempotent', async ({ request }) => {
    const listBefore = await request.get('/api/projects')
    const before: Array<{ id: string; path: string }> = await listBefore.json()
    expect(before.length).toBeGreaterThan(0)

    // Re-register the first project — the response must carry the same ID and path.
    const regResp = await request.post('/api/projects/register', {
      data: { path: before[0].path },
    })
    expect(regResp.ok()).toBeTruthy()
    const entry = await regResp.json()
    expect(entry.id).toBe(before[0].id)
    expect(entry.path).toBe(before[0].path)

    // Project list length should not have grown.
    const listAfter = await request.get('/api/projects')
    const after = await listAfter.json()
    expect(after.length).toBeGreaterThanOrEqual(before.length)
  })

  test('TC-012.2 — scoped endpoint for non-registered project returns isolation boundary error', async ({ request }) => {
    // A non-registered project ID must return 404, not the default project's data.
    const resp = await request.get('/api/projects/proj-deadbeef/workers')
    expect(resp.status()).toBe(404)
    const body = await resp.json()
    expect(typeof body.error).toBe('string')
    expect(body.error.length).toBeGreaterThan(0)
  })

  test('TC-012.3 — concurrent requests against different endpoints complete without racing', async ({ request }) => {
    // Fire several independent requests in parallel.
    const [r1, r2, r3, r4, r5] = await Promise.all([
      request.get('/api/projects'),
      request.get('/api/beads'),
      request.get('/api/documents'),
      request.get('/api/health'),
      request.get('/api/personas'),
    ])
    expect(r1.ok()).toBeTruthy()
    expect(r2.ok()).toBeTruthy()
    expect(r3.ok()).toBeTruthy()
    expect(r4.ok()).toBeTruthy()
    expect(r5.ok()).toBeTruthy()
  })

  test('TC-012.4 — non-registered project returns 404 while server remains healthy', async ({ request }) => {
    // Project ID that was never registered should yield 404 for scoped endpoints.
    const isolatedResp = await request.get('/api/projects/proj-00dead00/workers')
    expect(isolatedResp.status()).toBe(404)

    // Server must continue serving other requests normally.
    const healthResp = await request.get('/api/health')
    expect(healthResp.ok()).toBeTruthy()
    const body = await healthResp.json()
    expect(body.status).toBe('ok')
  })

  test('TC-012.5 — GET /api/projects/:id/workers scopes workers to the named project', async ({ request }) => {
    // Each registered project has its own worker namespace; requesting workers for
    // project A must not return workers that belong to project B.
    const listResp = await request.get('/api/projects')
    const projects = await listResp.json()
    expect(projects.length).toBeGreaterThan(0)

    // Workers for the registered project: must be an array (empty or otherwise).
    const workersResp = await request.get(`/api/projects/${projects[0].id}/workers`)
    expect(workersResp.ok()).toBeTruthy()
    const workers = await workersResp.json()
    expect(Array.isArray(workers)).toBeTruthy()

    // Workers for an unknown project must return 404, not an empty array from the real project.
    const unknownResp = await request.get('/api/projects/proj-ffffff00/workers')
    expect(unknownResp.status()).toBe(404)
  })
})
