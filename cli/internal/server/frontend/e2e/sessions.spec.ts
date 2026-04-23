import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/sessions`;

type SessionNode = {
	id: string;
	projectId: string;
	beadId: string | null;
	harness: string;
	model: string;
	effort: string;
	status: string;
	startedAt: string;
	endedAt: string | null;
	durationMs: number;
	cost: number | null;
	tokens: { prompt: number; completion: number; total: number; cached: number };
	outcome: string;
	detail: string | null;
};

const olderSession: SessionNode = {
	id: 'sess-older-20260418',
	projectId: PROJECT_ID,
	beadId: 'ddx-old',
	harness: 'claude',
	model: 'claude-sonnet-4-6',
	effort: 'standard',
	status: 'completed',
	startedAt: '2026-04-18T10:00:00Z',
	endedAt: '2026-04-18T10:00:03Z',
	durationMs: 3000,
	cost: 0.01,
	tokens: { prompt: 100, completion: 50, total: 150, cached: 0 },
	outcome: 'success',
	detail: null
};

const latestBundleSession: SessionNode = {
	id: 'sess-latest-20260422',
	projectId: PROJECT_ID,
	beadId: 'ddx-new',
	harness: 'codex',
	model: 'gpt-5.4',
	effort: 'high',
	status: 'completed',
	startedAt: '2026-04-22T12:00:00Z',
	endedAt: '2026-04-22T12:00:04Z',
	durationMs: 4000,
	cost: 0.02,
	tokens: { prompt: 200, completion: 80, total: 280, cached: 20 },
	outcome: 'success',
	detail: null
};

const liveSession: SessionNode = {
	id: 'sess-live-20260422',
	projectId: PROJECT_ID,
	beadId: 'ddx-live',
	harness: 'agent',
	model: 'qwen3.6',
	effort: 'medium',
	status: 'completed',
	startedAt: '2026-04-22T12:01:00Z',
	endedAt: '2026-04-22T12:01:02Z',
	durationMs: 2000,
	cost: null,
	tokens: { prompt: 300, completion: 120, total: 420, cached: 0 },
	outcome: 'success',
	detail: null
};

function sessionsPayload(rows: SessionNode[]) {
	return {
		agentSessions: {
			edges: rows.map((node) => ({ node, cursor: node.id })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: rows.length
		}
	};
}

test('sessions page lists sharded sessions, lazy-loads bundle bodies, and refreshes live', async ({ page }) => {
	let includeLiveSession = false;
	let detailRequested = false;

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
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
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: sessionsPayload(rows) }) });
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
	await expect(page.getByText('codex')).toBeVisible();
	await expect(page.getByText('gpt-5.4')).toBeVisible();
	await expect(page.getByText(/No sessions recorded between/)).toBeVisible();
	await expect(page.getByRole('row', { name: /codex.*gpt-5\.4.*4\/22\/2026/i })).toBeVisible();

	await page.getByRole('row', { name: /codex.*gpt-5\.4/i }).click();
	await expect.poll(() => detailRequested).toBe(true);
	await expect(page.getByText('bundle prompt body')).toBeVisible();
	await expect(page.getByText('bundle response body')).toBeVisible();

	includeLiveSession = true;
	await expect(page.getByRole('row', { name: /agent.*qwen3\.6/i })).toBeVisible({ timeout: 3500 });
});
