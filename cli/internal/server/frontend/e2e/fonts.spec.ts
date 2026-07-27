import { expect, test } from '@playwright/test';

// Documents is an already-covered functional route whose h1 uses
// font-headline-md (Inter Variable stack).
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/documents`;

const DOCUMENTS = [
	{ id: 'doc-001', path: 'docs/helix/01-frame/vision.md', title: 'Vision' },
	{ id: 'doc-002', path: 'docs/helix/01-frame/prd.md', title: 'PRD' }
];

const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];

async function mockRoutes(page: import('@playwright/test').Page) {
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
		} else if (body.query.includes('Documents')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						documents: {
							edges: DOCUMENTS.map((d, i) => ({ node: d, cursor: `cursor-${i}` })),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: DOCUMENTS.length
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});
}

test('TestInterActuallyLoadsInBrowser', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(BASE_URL);
	await page.waitForSelector('h1');

	await page.evaluate(() => document.fonts.ready);

	const faceLoaded = await page.evaluate(() =>
		document.fonts.check('700 11px "Inter Variable"')
	);
	expect(faceLoaded, 'Inter Variable face must be loaded (not merely declared)').toBe(true);

	const fontFamily = await page.evaluate(() => {
		const h1 = document.querySelector('h1');
		if (!h1) return '';
		return getComputedStyle(h1).fontFamily;
	});
	expect(fontFamily, 'h1 computed font-family must include Inter Variable').toContain(
		'Inter Variable'
	);
});
