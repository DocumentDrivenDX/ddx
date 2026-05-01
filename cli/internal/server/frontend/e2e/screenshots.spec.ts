import { test, expect } from '@playwright/test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Marketing assets — feature-area screenshots referenced by
// website/content/features/_index.md. Written directly into
// website/static/ui/ with the exact filenames the Hugo site renders.
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const WEBSITE_UI_DIR = path.resolve(__dirname, '../../../../../website/static/ui');

// DDx Server UI — visual regression screenshots
// These capture each page for visual review and regression detection.
// Run: bunx playwright test e2e/screenshots.spec.ts --update-snapshots
// to update baselines after intentional changes.
//
// Dark/light parity is a FEAT-008 frontend-design gate
// (docs/helix/01-frame/concerns.md#frontend-design). Every page is
// snapshotted in both modes; any theme-specific palette drift fails CI.
//
// Per-project routes resolve their node/project IDs at runtime against
// the fixture-backed Go DDx server (see playwright.config.ts) so the
// screenshots render seeded fixture data rather than the developer's
// live .ddx/ state.

// Resolve fixture node + project IDs from the live harness. The fixture
// project is the workspace under a `mktemp -d -t ddx-e2e-XXXXXX` path —
// other entries in /api/projects (carried over from developer-local state)
// must not be picked up here.
async function getFixtureIds(
	request: import('@playwright/test').APIRequestContext
): Promise<{ nodeId: string; projectId: string }> {
	const nodeResp = await request.post('/graphql', {
		data: { query: '{ nodeInfo { id name } }' }
	});
	const nodeBody = (await nodeResp.json()) as {
		data: { nodeInfo: { id: string; name: string } };
	};
	const projectsResp = await request.get('/api/projects');
	const projects = (await projectsResp.json()) as Array<{ id: string; name: string; path: string }>;
	const fixture = projects.find((p) => /(^|\/)ddx-e2e-/.test(p.path) || /^ddx-e2e-/.test(p.name));
	if (!fixture) {
		throw new Error(
			`fixture server has no ddx-e2e-* project registered (got: ${projects
				.map((p) => p.id)
				.join(', ')})`
		);
	}
	return { nodeId: nodeBody.data.nodeInfo.id, projectId: fixture.id };
}

type PageSpec = {
	name: string;
	ready: string;
	maskStarted?: boolean;
	tolerance: number;
	// Path is resolved per-test so per-project routes can reference the
	// fixture-derived node/project IDs.
	path: (ids: { nodeId: string; projectId: string }) => string;
};

const PAGES: readonly PageSpec[] = [
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}`,
		name: 'dashboard',
		ready: 'h1',
		maskStarted: true,
		tolerance: 0.02
	},
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/beads`,
		name: 'beads-kanban',
		ready: 'text=OPEN',
		tolerance: 0.04
	},
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/documents`,
		name: 'documents',
		ready: 'h1',
		tolerance: 0.02
	},
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/graph`,
		name: 'graph',
		ready: 'h1',
		tolerance: 0.06
	},
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/sessions`,
		name: 'agent',
		ready: 'h1',
		tolerance: 0.04
	},
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/personas`,
		name: 'personas',
		ready: 'text=Personas',
		tolerance: 0.04
	},
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}`,
		name: 'project-overview',
		ready: 'text=Actions',
		tolerance: 0.04
	},
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/plugins`,
		name: 'plugins',
		ready: 'text=Plugins',
		tolerance: 0.04
	},
	{
		// The fixture workspace ships only the `ddx` plugin (see
		// e2e/fixtures/.ddx/plugins/), so the plugin-detail snapshot drives
		// that page rather than a developer-local plugin like `helix`.
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/plugins/ddx`,
		name: 'plugin-detail',
		ready: 'text=Manifest',
		tolerance: 0.04
	},
	{
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/efficacy`,
		name: 'efficacy',
		ready: 'text=Efficacy',
		tolerance: 0.04
	}
] as const;

const MODES = ['light', 'dark'] as const;

test.describe('DDx Server UI Screenshots', () => {
	for (const mode of MODES) {
		for (const pg of PAGES) {
			test(`${pg.name} (${mode})`, async ({ page, request }) => {
				const ids = await getFixtureIds(request);

				await page.addInitScript((m) => {
					window.localStorage.setItem('mode-watcher-mode', m);
				}, mode);

				await page.goto(pg.path(ids));
				await page.waitForSelector(pg.ready);
				await page.waitForTimeout(500);

				await expect(page).toHaveScreenshot(`${pg.name}-${mode}.png`, {
					fullPage: true,
					maxDiffPixelRatio: pg.tolerance,
					mask: pg.maskStarted ? [page.locator('text=/^Started:/')] : undefined
				});
			});
		}
	}
});

// ---------------------------------------------------------------------------
// Feature-area marketing screenshots — one PNG per feature in
// website/content/features/_index.md, written to website/static/ui/.
// These are not visual-regression baselines; they ship as the feature-page
// imagery on the public site. Filenames match the markdown image refs.
// ---------------------------------------------------------------------------

type FeatureArea = {
	id: string;
	file: string;
	ready: string;
	path: (ids: { nodeId: string; projectId: string }) => string;
	prepare?: (page: import('@playwright/test').Page) => Promise<void>;
};

const FEATURE_AREAS: readonly FeatureArea[] = [
	{
		id: 'artifact-graph',
		file: 'feature-artifact-graph.png',
		ready: 'h1',
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/graph`
	},
	{
		id: 'beads-dag',
		file: 'feature-beads-dag.png',
		ready: 'text=OPEN',
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/beads`,
		prepare: async (page) => {
			const card = page.locator('[draggable="true"]').first();
			if (await card.isVisible({ timeout: 3000 }).catch(() => false)) {
				await card.click();
				await page.waitForTimeout(800);
			}
		}
	},
	{
		id: 'execute-loop',
		file: 'feature-execute-loop.png',
		ready: 'h1',
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/workers`
	},
	{
		id: 'evidence-capture',
		file: 'feature-evidence-capture.png',
		ready: 'h1',
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/executions`
	},
	{
		id: 'multi-model-review',
		file: 'feature-multi-model-review.png',
		ready: 'h1',
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/runs`
	},
	{
		id: 'skills',
		file: 'feature-skills.png',
		ready: 'text=Plugins',
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/plugins`
	},
	{
		id: 'agent-dispatch',
		file: 'feature-agent-dispatch.png',
		ready: 'h1',
		path: ({ nodeId, projectId }) => `/nodes/${nodeId}/projects/${projectId}/sessions`
	}
] as const;

test.describe('DDx feature-area marketing screenshots', () => {
	for (const area of FEATURE_AREAS) {
		test(`feature: ${area.id}`, async ({ page, request }) => {
			const ids = await getFixtureIds(request);

			// Light mode is the canonical look on the marketing site.
			await page.addInitScript(() => {
				window.localStorage.setItem('mode-watcher-mode', 'light');
			});

			await page.goto(area.path(ids));
			await page.waitForSelector(area.ready);
			await page.waitForLoadState('networkidle').catch(() => {});
			if (area.prepare) await area.prepare(page);
			await page.waitForTimeout(500);

			await page.screenshot({
				path: path.join(WEBSITE_UI_DIR, area.file),
				fullPage: true
			});
		});
	}
});
