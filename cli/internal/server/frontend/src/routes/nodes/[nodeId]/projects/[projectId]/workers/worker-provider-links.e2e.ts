import { expect, test, type Page } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT = { id: 'proj-1', name: 'Project Alpha', path: '/repos/alpha' };

type GqlBody = { query: string; variables?: Record<string, unknown> };

const WORKERS = [
	{
		id: 'worker-001',
		kind: 'work',
		state: 'running',
		status: 'running',
		harness: 'claude',
		model: 'gpt-5.4',
		currentBead: 'bead-001',
		attempts: 3,
		successes: 2,
		failures: 1,
		startedAt: '2026-05-03T09:00:00Z'
	}
];

const TREND_7D = {
	name: 'claude',
	kind: 'HARNESS',
	windowDays: 7,
	ceilingTokens: 80000,
	projectedRunOutHours: null,
	series: [
		{
			bucketStart: '2026-05-03T00:00:00Z',
			tokens: 1200,
			requests: 3
		}
	]
};

const TREND_30D = {
	name: 'claude',
	kind: 'HARNESS',
	windowDays: 30,
	ceilingTokens: null,
	projectedRunOutHours: null,
	series: [
		{
			bucketStart: '2026-04-03T00:00:00Z',
			tokens: 520,
			requests: 1
		}
	]
};

const RECENT_USAGE_ROWS = [
	{
		id: 'session-001',
		startedAt: '2026-05-03T10:00:00Z',
		durationMs: 12500,
		harness: 'claude',
		provider: 'claude',
		model: 'claude-sonnet-4-6',
		effort: 'normal',
		status: 'completed',
		detail: 'finished successfully'
	}
];

async function mockGraphQL(page: Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as GqlBody;
		const q = body.query;

		if (q.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (q.includes('ProjectsForLayout')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						projects: {
							edges: [{ node: PROJECT }]
						}
					}
				})
			});
			return;
		}
		if (q.includes('WorkersByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						workersByProject: {
							edges: WORKERS.map((node, index) => ({
								node,
								cursor: `worker-${index}`
							})),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: WORKERS.length
						},
						queueAndWorkersSummary: { maxCount: null }
					}
				})
			});
			return;
		}
		if (q.includes('ProviderTrend')) {
			const windowDays = Number(body.variables?.windowDays ?? 7);
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerTrend: windowDays === 7 ? TREND_7D : TREND_30D
					}
				})
			});
			return;
		}
		if (q.includes('ProviderRecentUsage')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						agentSessions: {
							edges: RECENT_USAGE_ROWS.map((node) => ({ node })),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: RECENT_USAGE_ROWS.length
						}
					}
				})
			});
			return;
		}

		await route.continue();
	});

	await page.route('/api/projects', async (route) => {
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify([PROJECT])
		});
	});
}

test('worker rows link to provider detail pages', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/nodes/node-abc/projects/proj-1/workers');

	await expect(page.getByTestId('worker-provider-link-worker-001')).toHaveAttribute(
		'href',
		'/nodes/node-abc/providers/claude'
	);

	await page.getByTestId('worker-provider-link-worker-001').click();
	await expect(page).toHaveURL('/nodes/node-abc/providers/claude');
	await expect(page.getByTestId('provider-trend')).toBeVisible();
	await expect(page.getByTestId('recent-usage')).toBeVisible();
});
