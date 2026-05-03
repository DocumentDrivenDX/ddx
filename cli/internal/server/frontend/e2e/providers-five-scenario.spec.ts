import { expect, test } from '@playwright/test';

// Five-provider scenario: claude, codex, openrouter, lmstudio @vidar,
// lmstudio @bragi. Mocks GraphQL responses for ProviderStatuses,
// ProviderModels (per provider) and the RefreshProviderModels mutation
// so the page can be exercised without a live backend.

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECTS = [{ id: 'proj-1', name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = '/nodes/node-abc/providers';

interface MockProvider {
	name: string;
	kind: 'ENDPOINT' | 'HARNESS';
	providerType: string;
	baseURL: string;
	model: string;
	status: string;
	reachable: boolean;
	detail: string;
	modelCount: number;
	isDefault: boolean;
	cooldownUntil: string | null;
	lastCheckedAt: string;
	defaultForProfile: string[];
	usage: {
		tokensUsedLastHour: number;
		tokensUsedLast24h: number;
		requestsLastHour: number;
		requestsLast24h: number;
	};
	quota: null;
	sparkline: number[];
}

const PROVIDERS: MockProvider[] = [
	{
		name: 'claude',
		kind: 'HARNESS',
		providerType: 'anthropic',
		baseURL: '(api)',
		model: 'claude-sonnet-4-6',
		status: 'api key configured',
		reachable: true,
		detail: 'api key configured',
		modelCount: 4,
		isDefault: true,
		cooldownUntil: null,
		lastCheckedAt: '2026-05-03T12:00:00Z',
		defaultForProfile: ['default'],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null,
		sparkline: []
	},
	{
		name: 'codex',
		kind: 'HARNESS',
		providerType: 'openai',
		baseURL: '(api)',
		model: 'gpt-5',
		status: 'api key configured',
		reachable: true,
		detail: 'api key configured',
		modelCount: 3,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-05-03T12:00:00Z',
		defaultForProfile: [],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null,
		sparkline: []
	},
	{
		name: 'openrouter',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'https://openrouter.ai/api/v1',
		model: '',
		status: 'connected (200 models)',
		reachable: true,
		detail: 'connected',
		modelCount: 200,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-05-03T12:00:00Z',
		defaultForProfile: [],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null,
		sparkline: []
	},
	{
		name: 'lmstudio @vidar',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://vidar.local:1234/v1',
		model: '',
		status: 'connected (3 models)',
		reachable: true,
		detail: 'connected',
		modelCount: 3,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-05-03T12:00:00Z',
		defaultForProfile: [],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null,
		sparkline: []
	},
	{
		name: 'lmstudio @bragi',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://bragi.local:1234/v1',
		model: '',
		status: 'connected (2 models)',
		reachable: true,
		detail: 'connected',
		modelCount: 2,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-05-03T12:00:00Z',
		defaultForProfile: [],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null,
		sparkline: []
	}
];

const MODELS_BY_KEY: Record<string, { id: string; contextLength: number | null; available: boolean }[]> = {
	'HARNESS|claude': [
		{ id: 'claude-sonnet-4-6', contextLength: 200000, available: true },
		{ id: 'claude-opus-4-7', contextLength: 200000, available: true },
		{ id: 'claude-haiku-4-5', contextLength: 200000, available: true },
		{ id: 'claude-haiku-3-5', contextLength: 200000, available: true }
	],
	'HARNESS|codex': [
		{ id: 'gpt-5', contextLength: 256000, available: true },
		{ id: 'gpt-5-mini', contextLength: 128000, available: true },
		{ id: 'o3-mini', contextLength: 128000, available: true }
	],
	'ENDPOINT|openrouter': Array.from({ length: 200 }, (_, i) => ({
		id: `openrouter/model-${i + 1}`,
		contextLength: 32768,
		available: true
	})),
	'ENDPOINT|lmstudio @vidar': [
		{ id: 'qwen2.5-coder-32b-instruct', contextLength: 32768, available: true },
		{ id: 'llama-3.1-70b', contextLength: 8192, available: true },
		{ id: 'mistral-7b', contextLength: 8192, available: true }
	],
	'ENDPOINT|lmstudio @bragi': [
		{ id: 'qwen2.5-coder-7b', contextLength: 32768, available: true },
		{ id: 'phi-4', contextLength: 16384, available: true }
	]
};

const DEFAULT_ROUTE = {
	modelRef: 'code-large',
	resolvedProvider: 'claude',
	resolvedModel: 'claude-sonnet-4-6',
	strategy: 'first-available'
};

interface RefreshCounter {
	count: number;
	lastVariables: Record<string, unknown> | null;
}

async function mockGraphQL(
	page: import('@playwright/test').Page,
	refreshCounter: RefreshCounter
) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
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
		} else if (body.query.includes('ProviderStatuses')) {
			const endpoints = PROVIDERS.filter((p) => p.kind === 'ENDPOINT');
			const harnesses = PROVIDERS.filter((p) => p.kind === 'HARNESS');
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerStatuses: endpoints,
						harnessStatuses: harnesses,
						defaultRouteStatus: DEFAULT_ROUTE
					}
				})
			});
		} else if (body.query.includes('DefaultRouteStatus')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { defaultRouteStatus: DEFAULT_ROUTE } })
			});
		} else if (body.query.includes('RefreshProviderModels')) {
			refreshCounter.count += 1;
			refreshCounter.lastVariables = body.variables ?? null;
			const name = String(body.variables?.name ?? '');
			const kind = String(body.variables?.kind ?? '');
			const key = `${kind}|${name}`;
			const models = MODELS_BY_KEY[key] ?? [];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						refreshProviderModels: {
							name,
							kind,
							baseURL: '',
							fetchedAt: '2026-05-03T12:05:00Z',
							fromCache: false,
							models
						}
					}
				})
			});
		} else if (body.query.includes('ProviderModels')) {
			const name = String(body.variables?.name ?? '');
			const kind = String(body.variables?.kind ?? '');
			const key = `${kind}|${name}`;
			const models = MODELS_BY_KEY[key] ?? [];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerModels: {
							name,
							kind,
							baseURL: '',
							fetchedAt: '2026-05-03T12:00:00Z',
							fromCache: true,
							models
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});
}

test('TC-068: 5-provider scenario renders all providers with model lists', async ({ page }) => {
	const refreshCounter: RefreshCounter = { count: 0, lastVariables: null };
	await mockGraphQL(page, refreshCounter);
	await page.goto(BASE_URL);

	// All 5 providers visible in the table.
	for (const p of PROVIDERS) {
		await expect(page.getByTestId(`endpoint-row-${p.name}`)).toBeVisible();
	}

	// Counts widget reflects the split.
	await expect(
		page.getByText(/5 total \(3 endpoints · 2 harnesses\)/)
	).toBeVisible();

	// Expand each row and assert the inline model list renders with at least
	// one expected model id from the per-provider fixture.
	const expectations: Array<{ provider: string; sample: string }> = [
		{ provider: 'claude', sample: 'claude-sonnet-4-6' },
		{ provider: 'codex', sample: 'o3-mini' },
		{ provider: 'openrouter', sample: 'openrouter/model-1' },
		{ provider: 'lmstudio @vidar', sample: 'qwen2.5-coder-32b-instruct' },
		{ provider: 'lmstudio @bragi', sample: 'qwen2.5-coder-7b' }
	];

	for (const { provider, sample } of expectations) {
		await page.getByTestId(`endpoint-models-toggle-${provider}`).click();
		const list = page.getByTestId(`endpoint-models-list-${provider}`);
		await expect(list).toBeVisible();
		await expect(list.getByText(sample, { exact: true })).toBeVisible();
	}

	// Drilldown link is shown for the truncated openrouter list (200 > 10).
	await expect(
		page.getByTestId('endpoint-models-drilldown-openrouter')
	).toBeVisible();
});

test('TC-069: per-row refresh button drives a re-query', async ({ page }) => {
	const refreshCounter: RefreshCounter = { count: 0, lastVariables: null };
	await mockGraphQL(page, refreshCounter);
	await page.goto(BASE_URL);

	await expect(page.getByTestId('endpoint-row-claude')).toBeVisible();
	expect(refreshCounter.count).toBe(0);

	await page.getByTestId('endpoint-models-refresh-claude').click();

	await expect
		.poll(() => refreshCounter.count, { timeout: 5_000 })
		.toBeGreaterThanOrEqual(1);
	expect(refreshCounter.lastVariables).toMatchObject({ name: 'claude', kind: 'HARNESS' });

	// Refreshed list is rendered (refreshModels auto-expands the row).
	const list = page.getByTestId('endpoint-models-list-claude');
	await expect(list).toBeVisible();
	await expect(list.getByText('claude-sonnet-4-6', { exact: false })).toBeVisible();

	// A second provider's refresh button works too and routes the right
	// variables (kind = ENDPOINT for openrouter).
	await page.getByTestId('endpoint-models-refresh-openrouter').click();
	await expect
		.poll(() => refreshCounter.count, { timeout: 5_000 })
		.toBeGreaterThanOrEqual(2);
	expect(refreshCounter.lastVariables).toMatchObject({
		name: 'openrouter',
		kind: 'ENDPOINT'
	});
});
