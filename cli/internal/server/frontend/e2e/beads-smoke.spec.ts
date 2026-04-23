// Playwright smoke test for ddx-9ce6842a AC §8.
//
// These tests measure wall-clock time from page.goto to "list is
// interactive" against a mocked GraphQL layer that returns a realistic-
// sized payload (50 beads — the default page window). They are ceilings,
// not p95 targets: the gate catches outright regressions (runaway render
// loops, missing virtualization, blocking scripts) without tying the
// repo's green/red status to machine speed.
//
// The matching server-side ceilings (GraphQL response under the same
// budget on the real fixture) are enforced in
// cli/internal/server/perf/smoke_test.go — together they guard the full
// round-trip the user feels.
import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-smoke', name: 'smoke node' };
const PROJECT_ID = 'proj-smoke';
const PROJECTS = [{ id: PROJECT_ID, name: 'Smoke project', path: '/repos/smoke' }];

function generateBeads(count: number) {
	const beads = [];
	for (let i = 0; i < count; i++) {
		beads.push({
			id: `ddx-smoke-${String(i).padStart(4, '0')}`,
			title: `Smoke fixture bead ${i}`,
			status: i % 3 === 0 ? 'closed' : 'open',
			priority: i % 4,
			labels: ['smoke', `bucket-${i % 7}`]
		});
	}
	return beads;
}

async function mockSmokeGraphQL(page: import('@playwright/test').Page, beadCount: number) {
	const beads = generateBeads(beadCount);
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('BeadsByProject') || body.query.includes('beadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						beadsByProject: {
							edges: beads.map((b, i) => ({ node: b, cursor: `cursor-${i}` })),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: beads.length
						}
					}
				})
			});
		} else if (body.query.includes('query Beads(') || body.query.includes('Beads(')) {
			// Cross-project list fetches the `beads` field.
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						beads: {
							edges: beads.map((b, i) => ({ node: b, cursor: `cursor-${i}` })),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: beads.length
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});
}

// ddx-9ce6842a AC §8: per-project /beads interactive within 1s.
test('smoke: /beads list is interactive within 1s on 50-bead fixture', async ({ page }) => {
	await mockSmokeGraphQL(page, 50);

	const start = Date.now();
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/beads`);
	// "Interactive" = the heading has rendered AND at least one bead row is
	// visible (so clicking it would navigate). Both are prerequisites to a
	// real user clicking anything.
	await expect(page.getByRole('heading', { name: 'Beads' })).toBeVisible({ timeout: 1000 });
	await expect(page.getByText('Smoke fixture bead 0')).toBeVisible({ timeout: 1000 });
	const elapsed = Date.now() - start;
	expect(elapsed, `per-project /beads interactive in ${elapsed}ms (ceiling 1000ms)`).toBeLessThan(
		1000
	);
});

// ddx-9ce6842a AC §8: cross-project /beads interactive within 2s.
// We drive the same list view with 300 rows to mimic the cross-project
// aggregate load — the backend ceiling is wider (2s) because the real call
// is beads() with no projectID.
test('smoke: cross-project /beads list is interactive within 2s on 300-bead fixture', async ({
	page
}) => {
	await mockSmokeGraphQL(page, 300);

	const start = Date.now();
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/beads`);
	await expect(page.getByRole('heading', { name: 'Beads' })).toBeVisible({ timeout: 2000 });
	await expect(page.getByText('Smoke fixture bead 0')).toBeVisible({ timeout: 2000 });
	const elapsed = Date.now() - start;
	expect(elapsed, `cross-project /beads interactive in ${elapsed}ms (ceiling 2000ms)`).toBeLessThan(
		2000
	);
});
