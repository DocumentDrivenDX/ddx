import { expect, test, type Page } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };

const ENDPOINT_ROWS = [
	{
		name: 'qwen-local',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://localhost:1234/v1',
		model: 'qwen3-7b',
		status: 'connected',
		reachable: true,
		detail: 'connected',
		modelCount: 3,
		isDefault: true,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-23T12:00:00Z',
		defaultForProfile: ['default'],
		usage: {
			tokensUsedLastHour: 12000,
			tokensUsedLast24h: 300000,
			requestsLastHour: 8,
			requestsLast24h: 220
		},
		quota: null
	}
];

const HARNESS_ROWS = [
	{
		name: 'claude',
		kind: 'HARNESS',
		providerType: 'subprocess',
		baseURL: '(subprocess)',
		model: 'claude-sonnet-4-6',
		status: 'available',
		reachable: true,
		detail: '/usr/local/bin/claude',
		modelCount: 0,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-23T12:00:00Z',
		defaultForProfile: [],
		usage: {
			tokensUsedLastHour: 5000,
			tokensUsedLast24h: 80000,
			requestsLastHour: 4,
			requestsLast24h: 65
		},
		quota: {
			ceilingTokens: 80000,
			ceilingWindowSeconds: 60,
			remaining: 75000,
			resetAt: '2026-04-23T12:01:00Z'
		}
	},
	{
		name: 'codex',
		kind: 'HARNESS',
		providerType: 'subprocess',
		baseURL: '(subprocess)',
		model: 'gpt-5.4',
		status: 'available',
		reachable: true,
		detail: '/usr/local/bin/codex',
		modelCount: 0,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-23T12:00:00Z',
		defaultForProfile: [],
		usage: {
			tokensUsedLastHour: 0,
			tokensUsedLast24h: 10000,
			requestsLastHour: 0,
			requestsLast24h: 12
		},
		quota: null
	}
];

const TREND_WITH_CEILING = {
	name: 'claude',
	kind: 'HARNESS',
	windowDays: 7,
	ceilingTokens: 80000,
	projectedRunOutHours: 16.2,
	series: Array.from({ length: 24 * 7 }, (_, i) => ({
		bucketStart: new Date(Date.UTC(2026, 3, 16 + Math.floor(i / 24), i % 24, 0, 0)).toISOString(),
		tokens: 1000 + i * 20,
		requests: 1 + Math.floor(i / 24)
	}))
};

const TREND_30D = {
	name: 'claude',
	kind: 'HARNESS',
	windowDays: 30,
	ceilingTokens: null,
	projectedRunOutHours: null,
	series: Array.from({ length: 6 * 30 }, (_, i) => ({
		bucketStart: new Date(Date.UTC(2026, 2, 24, i * 4, 0, 0)).toISOString(),
		tokens: 500 + i,
		requests: 1
	}))
};

async function mockGraphQL(page: Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: { name: string; windowDays: number };
		};
		const q = body.query;
		if (q.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (q.includes('ProviderStatuses')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerStatuses: ENDPOINT_ROWS,
						harnessStatuses: HARNESS_ROWS,
						defaultRouteStatus: null
					}
				})
			});
			return;
		}
		if (q.includes('ProviderTrend')) {
			const windowDays = body.variables?.windowDays ?? 7;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerTrend: windowDays === 7 ? TREND_WITH_CEILING : TREND_30D
					}
				})
			});
			return;
		}
		await route.continue();
	});
}

test('unified view shows endpoints and harnesses with kind labels', async ({ page }) => {
	await mockGraphQL(page);
	const start = Date.now();
	await page.goto('/nodes/node-abc/providers');
	// AC 1: table interactive within 500ms of navigation — since mocked
	// responses return instantly, asserting visibility of the table here is
	// a good proxy (real probes are async and out-of-band).
	await expect(page.getByTestId('agent-endpoints-table')).toBeVisible();
	const interactiveMs = Date.now() - start;
	expect(interactiveMs).toBeLessThan(500);

	await expect(page.getByTestId('endpoint-row-qwen-local')).toBeVisible();
	await expect(page.getByTestId('endpoint-row-claude')).toBeVisible();
	await expect(page.getByTestId('endpoint-row-codex')).toBeVisible();

	await expect(page.getByTestId('endpoint-kind-qwen-local')).toHaveText('endpoint');
	await expect(page.getByTestId('endpoint-kind-claude')).toHaveText('harness');
	await expect(page.getByTestId('endpoint-kind-codex')).toHaveText('harness');
	await expect(page.getByTestId('endpoint-reachable-claude')).toHaveText('reachable');

	// Tokens column populated for rows with usage.
	await expect(page.getByTestId('endpoint-tokens-qwen-local')).toContainText('12.0k');
	await expect(page.getByTestId('endpoint-tokens-claude')).toContainText('5.0k');
});

test('detail route renders 7d trend and projection callout', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/nodes/node-abc/providers/claude');

	await expect(page.getByTestId('provider-trend')).toBeVisible();
	await expect(page.getByTestId('series-7d')).toBeVisible();
	await expect(page.getByTestId('series-30d')).toBeVisible();
	await expect(page.getByTestId('projection-callout')).toContainText('Projected to hit quota');
});
