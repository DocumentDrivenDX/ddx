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
		quota: null,
		sparkline: [100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 1100, 1200, 1300, 1400, 1500, 1600, 1700, 1800, 1900, 2000, 2100, 2200, 2300, 2400]
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
		},
		sparkline: [50, 100, 150, 200, 250, 300, 350, 400, 450, 500, 550, 600, 650, 700, 750, 800, 850, 900, 950, 1000, 1050, 1100, 1150, 1200]
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
		quota: null,
		sparkline: [100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 1100, 1200, 1300, 1400, 1500, 1600, 1700, 1800, 1900, 2000, 2100, 2200, 2300, 2400]
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

const AGENT_METRICS_ROWS = [
	{
		key: 'claude',
		attempts: 42,
		successes: 36,
		successRate: 0.857,
		meanDurationMs: 12500,
		p50DurationMs: 9000,
		p95DurationMs: 28000,
		meanCostUsd: 0.014,
		effectiveCostPerSuccessUsd: 0.0163,
		meanInputTokens: 1200,
		meanOutputTokens: 450,
		lastSeenAt: '2026-05-03T11:00:00Z'
	},
	{
		key: 'codex',
		attempts: 17,
		successes: 12,
		successRate: 0.706,
		meanDurationMs: 19500,
		p50DurationMs: 15000,
		p95DurationMs: 41000,
		meanCostUsd: 0.022,
		effectiveCostPerSuccessUsd: 0.031,
		meanInputTokens: 1500,
		meanOutputTokens: 600,
		lastSeenAt: '2026-05-03T10:30:00Z'
	}
];

async function mockGraphQL(page: Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: { name: string; windowDays: number; window?: string };
		};
		const q = body.query;
		if (q.includes('AgentMetricsByProvider')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						agentMetrics: {
							window: body.variables?.window ?? 'W7D',
							groupBy: 'PROVIDER',
							revision: 'rev-test',
							rows: AGENT_METRICS_ROWS
						}
					}
				})
			});
			return;
		}
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
						harnessStatuses: HARNESS_ROWS
					}
				})
			});
			return;
		}
		if (q.includes('DefaultRouteStatus')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { defaultRouteStatus: null } })
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

	// Sparkline (24h) renders for rows with ≥6 hourly buckets of usage (AC 2).
	await expect(page.getByTestId('endpoint-sparkline-bars-qwen-local')).toBeVisible();
	await expect(page.getByTestId('endpoint-sparkline-bars-claude')).toBeVisible();
});

test('Availability/Performance section toggle drives URL and renders agentMetrics rows', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/nodes/node-abc/providers');

	// Both section chips are visible; default section is Availability.
	await expect(page.getByTestId('section-chip-availability')).toHaveAttribute('aria-pressed', 'true');
	await expect(page.getByTestId('section-chip-performance')).toHaveAttribute('aria-pressed', 'false');
	await expect(page.getByTestId('agent-endpoints-table')).toBeVisible();

	// Click Performance — URL gains ?section=performance and the perf table renders.
	await page.getByTestId('section-chip-performance').click();
	await expect(page).toHaveURL(/\?section=performance/);
	await expect(page.getByTestId('performance-table')).toBeVisible();
	await expect(page.getByTestId('performance-row-claude')).toBeVisible();
	await expect(page.getByTestId('performance-row-codex')).toBeVisible();

	// Drill-through link follows Story 8 chip query-param schema (k=v) and
	// targets the per-provider detail page.
	const link = page.getByTestId('performance-link-claude');
	await expect(link).toHaveAttribute('href', '/nodes/node-abc/providers/claude?window=7d');

	// Window chip switches via the same query-param schema.
	await page.getByTestId('performance-window-24h').click();
	await expect(page).toHaveURL(/section=performance/);
	await expect(page).toHaveURL(/window=W24H/);
	await expect(page.getByTestId('performance-link-claude')).toHaveAttribute(
		'href',
		'/nodes/node-abc/providers/claude?window=24h'
	);

	// Toggling back to Availability clears section param.
	await page.getByTestId('section-chip-availability').click();
	await expect(page).not.toHaveURL(/section=performance/);
	await expect(page.getByTestId('agent-endpoints-table')).toBeVisible();
});

test('detail route renders 7d trend and projection callout', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/nodes/node-abc/providers/claude');

	await expect(page.getByTestId('provider-trend')).toBeVisible();
	await expect(page.getByTestId('series-7d')).toBeVisible();
	await expect(page.getByTestId('series-30d')).toBeVisible();
	await expect(page.getByTestId('projection-callout')).toContainText('Projected to hit quota');
});
