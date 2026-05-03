import { expect, test } from '@playwright/test';

// Fixtures
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECTS = [{ id: 'proj-1', name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = '/nodes/node-abc/providers';

const PROVIDERS = [
	{
		name: 'claude',
		kind: 'ENDPOINT',
		providerType: 'anthropic',
		baseURL: '(api)',
		model: 'claude-sonnet-4-6',
		status: 'api key configured',
		reachable: true,
		detail: 'api key configured',
		modelCount: 0,
		isDefault: true,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-23T12:00:00Z',
		defaultForProfile: ['default'],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null
	},
	{
		name: 'local-qwen',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://localhost:1234/v1',
		model: 'qwen2.5-coder-32b-instruct',
		status: 'connected (5 models)',
		reachable: true,
		detail: 'connected',
		modelCount: 5,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-23T12:00:00Z',
		defaultForProfile: [],
		usage: {
			tokensUsedLastHour: 5000,
			tokensUsedLast24h: 20000,
			requestsLastHour: 2,
			requestsLast24h: 8
		},
		quota: null
	},
	{
		name: 'remote-llm',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://192.168.1.50:8080/v1',
		model: '',
		status: 'dial tcp: connection refused',
		reachable: false,
		detail: 'dial tcp: connection refused',
		modelCount: 0,
		isDefault: false,
		cooldownUntil: '2026-04-15T12:00:00Z',
		lastCheckedAt: '2026-04-23T12:00:00Z',
		defaultForProfile: [],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null
	}
];

const DEFAULT_ROUTE = {
	modelRef: 'code-medium',
	resolvedProvider: 'local-qwen',
	resolvedModel: 'qwen2.5-coder-32b-instruct',
	strategy: 'first-available'
};

async function mockGraphQL(page: import('@playwright/test').Page) {
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
		} else if (body.query.includes('ProviderStatuses')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerStatuses: PROVIDERS,
						harnessStatuses: [],
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
		} else {
			await route.continue();
		}
	});
}

// TC-060: Providers page loads with heading
test('TC-060: providers page loads with heading', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Agent availability' })).toBeVisible();
});

// TC-061: Provider table has expected columns
test('TC-061: provider table has expected columns', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: 'Kind' })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: 'Type' })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: 'Model', exact: true })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: 'Tokens (1h / 24h)' })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: 'Utilization' })).toBeVisible();
});

// TC-062: Provider rows render with correct data
test('TC-062: provider rows render with correct data', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// Provider names appear
	await expect(page.getByText('claude').first()).toBeVisible();
	await expect(page.getByText('local-qwen').first()).toBeVisible();
	await expect(page.getByText('remote-llm')).toBeVisible();

	// Provider types appear
	await expect(page.getByText('anthropic')).toBeVisible();
	await expect(page.getByText('openai-compat').first()).toBeVisible();

	// Status messages appear
	await expect(page.getByText('api key configured')).toBeVisible();
	await expect(page.getByText('connected (5 models)')).toBeVisible();
});

// TC-063: Default provider badge is shown
test('TC-063: default provider shows default badge', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// The 'default' badge should be visible for the default provider
	await expect(page.getByText('default').first()).toBeVisible();
});

// TC-064: Cooldown badge is shown for providers on cooldown
test('TC-064: cooldown badge appears for providers with active cooldown', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByText('cooldown')).toBeVisible();
});

// TC-065: Default route widget shows model-ref and resolved provider
test('TC-065: default route widget shows model-ref and resolved provider', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByText(/code-medium/)).toBeVisible();
	await expect(page.getByText(/local-qwen/).first()).toBeVisible();
	await expect(page.getByText(/first-available/)).toBeVisible();
});

// TC-066: Providers count is shown
test('TC-066: providers page shows configured count', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByText(/3 total \(3 endpoints · 0 harnesses\)/)).toBeVisible();
});

// TC-067: Empty state when no providers configured
test('TC-067: empty state shown when no providers returned', async ({ page }) => {
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
		} else if (body.query.includes('ProviderStatuses')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerStatuses: [],
						harnessStatuses: [],
						defaultRouteStatus: {
							modelRef: '',
							resolvedProvider: null,
							resolvedModel: null,
							strategy: null
						}
					}
				})
			});
		} else if (body.query.includes('DefaultRouteStatus')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						defaultRouteStatus: {
							modelRef: '',
							resolvedProvider: null,
							resolvedModel: null,
							strategy: null
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	await expect(page.getByText(/No agent endpoints configured/)).toBeVisible();
	await expect(page.getByText(/0 total/)).toBeVisible();
});
