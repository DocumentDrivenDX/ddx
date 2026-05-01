import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/sessions`;

type SessionNode = {
	id: string;
	projectId: string;
	beadId: string | null;
	workerId: string | null;
	harness: string;
	model: string;
	effort: string;
	status: string;
	startedAt: string;
	endedAt: string | null;
	durationMs: number;
	cost: number | null;
	billingMode: 'paid' | 'subscription' | 'local';
	tokens: { prompt: number; completion: number; total: number; cached: number };
	outcome: string;
	detail: string | null;
};

const olderSession: SessionNode = {
	id: 'sess-older-20260418',
	projectId: PROJECT_ID,
	beadId: 'ddx-old',
	workerId: null,
	harness: 'claude',
	model: 'claude-sonnet-4-6',
	effort: 'standard',
	status: 'completed',
	startedAt: '2026-04-18T10:00:00Z',
	endedAt: '2026-04-18T10:00:03Z',
	durationMs: 3000,
	cost: 0.01,
	billingMode: 'subscription',
	tokens: { prompt: 100, completion: 50, total: 150, cached: 0 },
	outcome: 'success',
	detail: null
};

const latestBundleSession: SessionNode = {
	id: 'sess-latest-20260422',
	projectId: PROJECT_ID,
	beadId: 'ddx-new',
	workerId: 'worker-session-owner',
	harness: 'codex',
	model: 'gpt-5.4',
	effort: 'high',
	status: 'completed',
	startedAt: '2026-04-22T12:00:00Z',
	endedAt: '2026-04-22T12:00:04Z',
	durationMs: 4000,
	cost: 0.02,
	billingMode: 'paid',
	tokens: { prompt: 200, completion: 80, total: 280, cached: 20 },
	outcome: 'success',
	detail: null
};

const liveSession: SessionNode = {
	id: 'sess-live-20260422',
	projectId: PROJECT_ID,
	beadId: 'ddx-live',
	workerId: null,
	harness: 'agent',
	model: 'qwen3.6',
	effort: 'medium',
	status: 'completed',
	startedAt: '2026-04-22T12:01:00Z',
	endedAt: '2026-04-22T12:01:02Z',
	durationMs: 2000,
	cost: null,
	billingMode: 'local',
	tokens: { prompt: 300, completion: 120, total: 420, cached: 0 },
	outcome: 'success',
	detail: null
};

function sessionsPayload(rows: SessionNode[]) {
	const cashUsd = rows
		.filter((row) => row.billingMode === 'paid')
		.reduce((sum, row) => sum + (row.cost ?? 0), 0);
	const subscriptionEquivUsd = rows
		.filter((row) => row.billingMode === 'subscription')
		.reduce((sum, row) => sum + (row.cost ?? 0), 0);
	const localRows = rows.filter((row) => row.billingMode === 'local');
	return {
		agentSessions: {
			edges: rows.map((node) => ({ node, cursor: node.id })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: rows.length
		},
		sessionsCostSummary: {
			cashUsd,
			subscriptionEquivUsd,
			localSessionCount: localRows.length,
			localEstimatedUsd: null
		}
	};
}

test('sessions page lists sharded sessions, lazy-loads bundle bodies, and refreshes live', async ({
	page
}) => {
	let includeLiveSession = false;
	let detailRequested = false;

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
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((node) => ({ node })) } } })
			});
		} else if (body.query.includes('AgentSessions') || body.query.includes('agentSessions')) {
			const rows = includeLiveSession
				? [liveSession, latestBundleSession, olderSession]
				: [latestBundleSession, olderSession];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: sessionsPayload(rows) })
			});
		} else if (body.query.includes('AgentSessionDetail') || body.query.includes('agentSession')) {
			detailRequested = true;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						agentSession: {
							id: latestBundleSession.id,
							prompt: 'bundle prompt body',
							response: 'bundle response body',
							stderr: ''
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Sessions' })).toBeVisible();
	await expect(page.getByText(/Sessions are immutable agent-run history/)).toBeVisible();
	await expect(page.getByRole('link', { name: 'Workers →' })).toHaveAttribute('href', /\/workers$/);
	await expect(page.getByText('codex', { exact: true })).toBeVisible();
	await expect(page.getByText('gpt-5.4')).toBeVisible();
	await expect(page.getByText('Cash paid')).toBeVisible();
	await expect(page.getByText('Subscription value')).toBeVisible();
	await expect(page.getByText('Local sessions')).toBeVisible();
	await expect(page.getByText('$0.02', { exact: true })).toBeVisible();
	await expect(page.getByText('$0.01', { exact: true })).toBeVisible();
	await expect(page.getByLabel(/cash: Billed by pay-per-token APIs/)).toBeVisible();
	await expect(page.getByLabel(/sub: Dollar-equivalent for tokens consumed/)).toBeVisible();
	await expect(page.getByText(/No sessions recorded between/)).toBeVisible();
	await expect(page.getByRole('row', { name: /codex.*gpt-5\.4.*4\/22\/2026/i })).toBeVisible();

	await page.getByText('Cash paid').hover();
	await expect(page.getByRole('tooltip', { name: /Billed by pay-per-token APIs/ })).toBeVisible();
	await page.getByText('Subscription value').hover();
	await expect(
		page.getByRole('tooltip', { name: /Dollar-equivalent for tokens consumed/ })
	).toBeVisible();
	await page.getByText('Local sessions').hover();
	await expect(page.getByRole('tooltip', { name: /Sessions served locally/ })).toBeVisible();

	await page.getByLabel(/cash: Billed by pay-per-token APIs/).hover();
	await expect(page.getByRole('tooltip', { name: /OpenRouter, direct API keys/ })).toBeVisible();

	await page.getByRole('row', { name: /codex.*gpt-5\.4/i }).click();
	await expect.poll(() => detailRequested).toBe(true);
	await expect(page.getByRole('link', { name: 'worker-session-owner' })).toHaveAttribute(
		'href',
		/\/workers\/worker-session-owner$/
	);
	await expect(page.getByText('bundle prompt body')).toBeVisible();
	await expect(page.getByText('bundle response body')).toBeVisible();

	includeLiveSession = true;
	await expect(page.getByRole('row', { name: /agent.*qwen3\.6/i })).toBeVisible({ timeout: 3500 });
	await expect(page.getByLabel(/local: Sessions served locally/)).toBeVisible();
});

test('sessions cost cards use zero-state and configured local estimates', async ({ page }) => {
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
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((node) => ({ node })) } } })
			});
		} else if (body.query.includes('agentSessions')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						agentSessions: {
							edges: [],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 0
						},
						sessionsCostSummary: {
							cashUsd: 0,
							subscriptionEquivUsd: 0,
							localSessionCount: 0,
							localEstimatedUsd: 0
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);
	await expect(page.getByLabel(/Cash paid/).getByText('—')).toBeVisible();
	await expect(page.getByLabel(/Subscription value/).getByText('—')).toBeVisible();
	await expect(page.getByLabel(/Local sessions/).getByText('0')).toBeVisible();

	await page.unroute('/graphql');
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
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((node) => ({ node })) } } })
			});
		} else if (body.query.includes('agentSessions')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						agentSessions: {
							edges: [{ node: liveSession, cursor: liveSession.id }],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 1
						},
						sessionsCostSummary: {
							cashUsd: 0,
							subscriptionEquivUsd: 0,
							localSessionCount: 1,
							localEstimatedUsd: 0.42
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});

	await page.reload();
	await expect(page.getByText('$0.42 est.')).toBeVisible();
});
