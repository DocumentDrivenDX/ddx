import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const PROJECT_BASE = `/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}`;

async function mockRoutes(page: import('@playwright/test').Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, string>;
		};

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
		} else if (body.query.includes('DocumentByPath') || body.query.includes('documentByPath')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						documentByPath: {
							id: 'helix.vision',
							path: body.variables?.path ?? '',
							title: 'Vision'
						}
					}
				})
			});
		} else if (body.query.includes('Artifacts')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						artifacts: {
							edges: [],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 0
						}
					}
				})
			});
		} else if (body.query.includes('ArtifactDetail')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						artifact: {
							id: 'doc:helix.vision',
							path: 'docs/helix/01-frame/vision.md',
							title: 'Vision',
							mediaType: 'text/markdown',
							sha256: null,
							staleness: 'fresh',
							description: null,
							updatedAt: null,
							ddxFrontmatter: null,
							content: '# Vision',
							typeDefinitions: [],
							generatedBy: null
						}
					}
				})
			});
		} else if (body.query.includes('RunExists')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: null } })
			});
		} else {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: {} })
			});
		}
	});
}

test('legacy project documents list redirects to markdown artifacts', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(`${PROJECT_BASE}/documents`);
	await expect(page).toHaveURL(
		new RegExp(`${PROJECT_BASE}/artifacts\\?mediaType=text%2Fmarkdown$`)
	);
	await expect(page.getByRole('heading', { name: 'Artifacts' })).toBeVisible();
});

test('legacy document detail redirects to the canonical artifact detail', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(`${PROJECT_BASE}/documents/docs/helix/01-frame/vision.md`);
	await expect(page).toHaveURL(new RegExp(`${PROJECT_BASE}/artifacts/doc%3Ahelix\\.vision$`));
	await expect(page.getByRole('heading', { name: 'Vision' }).first()).toBeVisible();
});
