import { expect, test, type Page } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const SESSIONS_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/sessions`;

async function installRunsMocks(page: Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((node) => ({ node })) } } })
			});
			return;
		}
		if (body.query.includes('ProjectRuns') || body.query.includes('runs')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						runs: {
							edges: [],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 0
						}
					}
				})
			});
			return;
		}
		await route.continue();
	});
}

test('sessions compatibility route redirects to the run layer', async ({ page }) => {
	await installRunsMocks(page);

	await page.goto(SESSIONS_URL);

	await expect(page).toHaveURL(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs?layer=run`);
	await expect(page.getByRole('heading', { name: 'Runs' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'run', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
});

test('sessions compatibility redirect preserves existing filters', async ({ page }) => {
	await installRunsMocks(page);

	await page.goto(`${SESSIONS_URL}?status=failure&layer=worker`);

	await expect(page).toHaveURL(
		new RegExp(
			`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs\\?(?=.*status=failure)(?=.*layer=run)`
		)
	);
	await expect(page.getByRole('heading', { name: 'Runs' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'failure', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(page.getByRole('button', { name: 'run', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
});
