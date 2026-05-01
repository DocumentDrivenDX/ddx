import { test } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const WEBSITE_UI_DIR = path.resolve(__dirname, '../../../../../website/static/ui');

// DDx Server UI — Demo Recording (TP-002 TC-009)
//
// Produces a polished video walkthrough of all 6 pages with real data
// interactions. Designed for embedding in the DDx microsite.
//
// Run:
//   bun run demo:record
// Output:
//   demo-output/ contains a .webm video file

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
	const projects = (await projectsResp.json()) as Array<{
		id: string;
		name: string;
		path: string;
	}>;
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

test.describe('DDx Server UI Demo', () => {
	test('full walkthrough', async ({ page, request }) => {
		const ids = await getFixtureIds(request);
		const base = `/nodes/${ids.nodeId}/projects/${ids.projectId}`;

		// ---------------------------------------------------------------
		// 1. Project Overview — overview of the project
		// ---------------------------------------------------------------
		await test.step('Project Overview — overview of the project', async () => {
			await page.goto(base);
			await page.waitForSelector('h1');
			await page.waitForLoadState('networkidle');
			await page.waitForTimeout(2500);
		});

		// ---------------------------------------------------------------
		// 2. Documents — browse and read a document
		// ---------------------------------------------------------------
		await test.step('Documents — browse and read a document', async () => {
			await page.goto(`${base}/documents`);
			await page.waitForSelector('h1');
			await page.waitForTimeout(1000);

			// Select the first document to show rendered markdown
			const firstDoc = page.locator('.overflow-auto button').first();
			if (await firstDoc.isVisible({ timeout: 3000 })) {
				await firstDoc.click();
				await page.waitForSelector('.prose', { timeout: 5000 });
				await page.waitForTimeout(2000);
			}

			// Demonstrate search
			const docSearch = page.locator('input[placeholder*="Search"]');
			if (await docSearch.isVisible()) {
				await docSearch.fill('persona');
				await page.waitForTimeout(1000);
				await docSearch.fill('');
				await page.waitForTimeout(500);
			}
		});

		// ---------------------------------------------------------------
		// 3. Beads — kanban board, search, detail, create
		// ---------------------------------------------------------------
		await test.step('Beads — kanban board, search, detail, create', async () => {
			await page.goto(`${base}/beads`);
			await page.waitForSelector('text=OPEN');
			await page.waitForTimeout(1500);

			// Search for beads (type="search" distinguishes it from the command palette input)
			const beadSearch = page.locator('input[type="search"]');
			if (await beadSearch.isVisible()) {
				await beadSearch.fill('open');
				await page.waitForTimeout(1200);
				await beadSearch.fill('');
				await page.waitForTimeout(600);
			}

			// Click a bead card to show detail panel
			const beadCard = page.locator('[draggable="true"]').first();
			if (await beadCard.isVisible({ timeout: 3000 }).catch(() => false)) {
				await beadCard.click();
				await page.waitForTimeout(2000);
			}
		});

		// ---------------------------------------------------------------
		// 4. Graph — document dependency visualization
		// ---------------------------------------------------------------
		await test.step('Graph — document dependency visualization', async () => {
			await page.goto(`${base}/graph`);
			await page.waitForSelector('h1');
			await page.waitForTimeout(2500);
		});

		// ---------------------------------------------------------------
		// 5. Workers — session history
		// ---------------------------------------------------------------
		await test.step('Workers — session history', async () => {
			await page.goto(`${base}/workers`);
			await page.waitForSelector('h1');
			await page.waitForTimeout(2000);
		});

		// ---------------------------------------------------------------
		// 6. Personas — browse and view a persona
		// ---------------------------------------------------------------
		await test.step('Personas — browse and view a persona', async () => {
			await page.goto(`${base}/personas`);
			await page.waitForSelector('text=Personas');
			await page.waitForTimeout(2000);

			const firstPersona = page.locator('.w-80 button').first();
			if (await firstPersona.isVisible({ timeout: 2000 }).catch(() => false)) {
				await firstPersona.click();
				await page.waitForTimeout(2000);
			}
		});

		// ---------------------------------------------------------------
		// 7. Back to Project Overview — closing shot
		// ---------------------------------------------------------------
		await test.step('Back to Project Overview — closing shot', async () => {
			await page.goto(base);
			await page.waitForSelector('h1');
			await page.waitForTimeout(2000);
		});
	});
});

// ---------------------------------------------------------------------------
// Per-feature demo videos — one short walkthrough per feature area in
// website/content/features/_index.md, saved as website/static/ui/feature-*.webm.
// Videos are forced on for this describe; each test's recording is copied to
// the marketing assets dir after the page closes so Playwright flushes the file.
// ---------------------------------------------------------------------------

type FeatureClip = {
	id: string;
	file: string;
	walk: (page: import('@playwright/test').Page, base: string) => Promise<void>;
};

const FEATURE_CLIPS: readonly FeatureClip[] = [
	{
		id: 'artifact-graph',
		file: 'feature-artifact-graph.webm',
		walk: async (page, base) => {
			await page.goto(`${base}/graph`);
			await page.waitForSelector('h1');
			await page.waitForTimeout(2500);
		}
	},
	{
		id: 'beads-dag',
		file: 'feature-beads-dag.webm',
		walk: async (page, base) => {
			await page.goto(`${base}/beads`);
			await page.waitForSelector('text=OPEN');
			await page.waitForTimeout(1200);
			const card = page.locator('[draggable="true"]').first();
			if (await card.isVisible({ timeout: 3000 }).catch(() => false)) {
				await card.click();
				await page.waitForTimeout(2000);
			}
		}
	},
	{
		id: 'execute-loop',
		file: 'feature-execute-loop.webm',
		walk: async (page, base) => {
			await page.goto(`${base}/workers`);
			await page.waitForSelector('h1');
			await page.waitForTimeout(2500);
		}
	},
	{
		id: 'evidence-capture',
		file: 'feature-evidence-capture.webm',
		walk: async (page, base) => {
			await page.goto(`${base}/executions`);
			await page.waitForSelector('h1');
			await page.waitForTimeout(2500);
		}
	},
	{
		id: 'multi-model-review',
		file: 'feature-multi-model-review.webm',
		walk: async (page, base) => {
			await page.goto(`${base}/runs`);
			await page.waitForSelector('h1');
			await page.waitForTimeout(2500);
		}
	},
	{
		id: 'skills',
		file: 'feature-skills.webm',
		walk: async (page, base) => {
			await page.goto(`${base}/plugins`);
			await page.waitForSelector('text=Plugins');
			await page.waitForTimeout(2000);
			const firstPlugin = page.locator('a[href*="/plugins/"]').first();
			if (await firstPlugin.isVisible({ timeout: 2000 }).catch(() => false)) {
				await firstPlugin.click();
				await page.waitForTimeout(2000);
			}
		}
	},
	{
		id: 'agent-dispatch',
		file: 'feature-agent-dispatch.webm',
		walk: async (page, base) => {
			await page.goto(`${base}/sessions`);
			await page.waitForSelector('h1');
			await page.waitForTimeout(2500);
		}
	}
] as const;

test.describe('DDx feature-area demo videos', () => {
	test.use({ video: 'on' });

	for (const clip of FEATURE_CLIPS) {
		test(`feature: ${clip.id}`, async ({ page, request }, testInfo) => {
			const ids = await getFixtureIds(request);
			const base = `/nodes/${ids.nodeId}/projects/${ids.projectId}`;

			await clip.walk(page, base);

			// Close the page so Playwright finalises the video file, then copy
			// it to website/static/ui/ under the marketing-asset filename.
			const video = page.video();
			await page.close();
			if (video) {
				const src = await video.path();
				const dst = path.join(WEBSITE_UI_DIR, clip.file);
				await fs.mkdir(path.dirname(dst), { recursive: true });
				await fs.copyFile(src, dst);
				await testInfo.attach(clip.file, { path: dst, contentType: 'video/webm' });
			}
		});
	}
});
