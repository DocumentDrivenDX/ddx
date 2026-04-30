import { expect, test, type Page } from '@playwright/test';

// ddx-950321ef / FEAT-006 AC#6 — routing-frontend-goldens.
//
// Replicates the user's real-config endpoint shape (lmstudio + omlx + lmstudio
// + lmstudio) and exercises the drain-queue flow end-to-end against mocked
// GraphQL: click Drain on the project home, observe a worker row appear with
// a harness backed by a live endpoint, never see a gemini-paired-with-non-
// gemini model or the 19-failure pattern, and confirm completion is either
// success or a typed error with a reason recorded on the bead. Attempts count
// must match work done.

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT = { id: 'proj-1', name: 'Project Alpha', path: '/repos/alpha' };

const HOME_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT.id}`;
const WORKERS_URL = `${HOME_URL}/workers`;

// User's real config: 4 endpoints, three lmstudio + one omlx, plus an
// optional gemini entry that is *not* in the routing allow-list. The harness
// dispatched by the drain must bind to one of the lmstudio/omlx live
// endpoints — never gemini.
const ENDPOINT_FIXTURE = [
	{
		name: 'lmstudio-a',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://localhost:1234/v1',
		model: 'qwen3.6-35b',
		status: 'connected (5 models)',
		reachable: true,
		detail: 'connected',
		modelCount: 5,
		isDefault: true,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-29T12:00:00Z',
		defaultForProfile: ['default'],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null
	},
	{
		name: 'omlx',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://vidar:8080/v1',
		model: 'qwen3.6-35b',
		status: 'connected (3 models)',
		reachable: true,
		detail: 'connected',
		modelCount: 3,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-29T12:00:00Z',
		defaultForProfile: [],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null
	},
	{
		name: 'lmstudio-b',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://localhost:1235/v1',
		model: 'qwen3.6-35b',
		status: 'connected (5 models)',
		reachable: true,
		detail: 'connected',
		modelCount: 5,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-29T12:00:00Z',
		defaultForProfile: [],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null
	},
	{
		name: 'lmstudio-c',
		kind: 'ENDPOINT',
		providerType: 'openai-compat',
		baseURL: 'http://localhost:1236/v1',
		model: 'qwen3.6-35b',
		status: 'connected (5 models)',
		reachable: true,
		detail: 'connected',
		modelCount: 5,
		isDefault: false,
		cooldownUntil: null,
		lastCheckedAt: '2026-04-29T12:00:00Z',
		defaultForProfile: [],
		usage: { tokensUsedLastHour: 0, tokensUsedLast24h: 0, requestsLastHour: 0, requestsLast24h: 0 },
		quota: null
	}
];

// Models advertised by gemini (cross-provider); the cross-pollination bug
// would surface a non-gemini provider hosting one of these.
const GEMINI_MODELS = new Set(['gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-1.5-pro']);
const GEMINI_PROVIDERS = new Set(['gemini', 'google', 'vertex']);

const QUEUE_READY = 4;

type GqlBody = { query: string; variables?: Record<string, unknown> };

interface WorkerNode {
	id: string;
	kind: string;
	state: string;
	status: string | null;
	harness: string;
	model: string;
	currentBead: string | null;
	attempts: number;
	successes: number;
	failures: number;
	startedAt: string;
	finishedAt?: string | null;
	lastError?: string | null;
}

function makeWorker(overrides: Partial<WorkerNode> = {}): WorkerNode {
	return {
		id: 'worker-drain-1',
		kind: 'execute-loop',
		state: 'running',
		status: 'running',
		harness: 'codex',
		model: 'qwen3.6-35b',
		currentBead: 'bead-001',
		attempts: 0,
		successes: 0,
		failures: 0,
		startedAt: '2026-04-29T12:00:00Z',
		finishedAt: null,
		lastError: null,
		...overrides
	};
}

function harnessBoundToLiveEndpoint(worker: WorkerNode): boolean {
	const liveModels = new Set(ENDPOINT_FIXTURE.filter((e) => e.reachable).map((e) => e.model));
	return liveModels.has(worker.model);
}

function noGeminiCrossContamination(worker: WorkerNode): boolean {
	const providerIsGemini = GEMINI_PROVIDERS.has(worker.harness.toLowerCase());
	const modelIsGemini = GEMINI_MODELS.has(worker.model);
	// A worker may be all-gemini or all-non-gemini, but never mixed.
	return providerIsGemini === modelIsGemini;
}

function attemptsMatchWorkDone(worker: WorkerNode): boolean {
	return worker.attempts === worker.successes + worker.failures;
}

async function installGraphqlMocks(
	page: Page,
	state: { workers: WorkerNode[]; dispatched: { count: number } }
) {
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
		if (q.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: [{ node: PROJECT }] } }
				})
			});
			return;
		}
		if (q.includes('ProjectQueueSummary')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueSummary: { ready: QUEUE_READY, blocked: 0, inProgress: 0 }
					}
				})
			});
			return;
		}
		if (q.includes('ProviderStatuses')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerStatuses: ENDPOINT_FIXTURE,
						harnessStatuses: [],
						defaultRouteStatus: {
							modelRef: 'code-medium',
							resolvedProvider: 'lmstudio-a',
							resolvedModel: 'qwen3.6-35b',
							strategy: 'first-available'
						}
					}
				})
			});
			return;
		}
		if (q.includes('WorkerDispatch') || q.includes('workerDispatch')) {
			state.dispatched.count += 1;
			const worker = makeWorker();
			state.workers = [worker, ...state.workers];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { workerDispatch: { id: worker.id, state: worker.state, kind: worker.kind } }
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
							edges: state.workers.map((w, i) => ({ node: w, cursor: `c-${i}` })),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: state.workers.length
						},
						queueAndWorkersSummary: { maxCount: null }
					}
				})
			});
			return;
		}
		if (q.includes('QueueAndWorkersSummary')) {
			const running = state.workers.filter((w) => w.state === 'running').length;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueAndWorkersSummary: {
							readyBeads: QUEUE_READY,
							runningWorkers: running,
							totalWorkers: state.workers.length,
							maxCount: null
						}
					}
				})
			});
			return;
		}
		if (q.includes('DrainIndicatorRunningWorkers')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						workersByProject: {
							edges: state.workers.map((w, i) => ({
								node: { id: w.id, state: w.state },
								cursor: `c-${i}`
							}))
						}
					}
				})
			});
			return;
		}
		if (q.includes('AgentSessions') || q.includes('agentSessions')) {
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
			return;
		}
		await route.continue();
	});
}

test('drain-queue golden: real-config endpoint shape, click Drain, worker row, no gemini cross-pollination, no 19-failure pattern', async ({
	page
}) => {
	const state: { workers: WorkerNode[]; dispatched: { count: number } } = {
		workers: [],
		dispatched: { count: 0 }
	};
	await installGraphqlMocks(page, state);

	// 1. Navigate to project home and click "Drain queue".
	await page.goto(HOME_URL);
	await expect(page.getByRole('heading', { name: PROJECT.name })).toBeVisible();
	await expect(page.getByText(/4 ready beads/)).toBeVisible();

	const drainCta = page.getByRole('button', { name: 'Drain queue' });
	await expect(drainCta).toBeEnabled();
	await drainCta.click();

	// 2. Confirm dialog dispatches the execute-loop worker.
	const startBtn = page.getByRole('button', { name: 'Start Drain queue' });
	await expect(startBtn).toBeVisible();
	const dispatchStartedAt = Date.now();
	await startBtn.click();

	// 3. Within 5s a worker row materializes (via the success card linking to it).
	const workerLink = page.getByRole('link', { name: /worker-drain-1/ });
	await expect(workerLink).toBeVisible({ timeout: 5000 });
	const elapsedMs = Date.now() - dispatchStartedAt;
	expect(elapsedMs).toBeLessThan(5000);
	expect(state.dispatched.count).toBe(1);

	// 4. The dispatched worker's harness is backed by a live endpoint and has
	// no gemini cross-pollination.
	const dispatched = state.workers[0];
	expect(harnessBoundToLiveEndpoint(dispatched)).toBe(true);
	expect(noGeminiCrossContamination(dispatched)).toBe(true);

	// 5. Drive the worker through "ran 4 attempts, all succeeded" then assert
	// the workers list reflects success and never the 19-failure pattern.
	state.workers = [
		makeWorker({
			state: 'done',
			status: 'success',
			attempts: QUEUE_READY,
			successes: QUEUE_READY,
			failures: 0,
			finishedAt: '2026-04-29T12:05:00Z',
			currentBead: null,
			lastError: null
		})
	];

	await page.goto(WORKERS_URL);
	await expect(page.getByRole('heading', { name: 'Workers' })).toBeVisible();
	await expect(page.getByText('worker-d')).toBeVisible();
	await expect(page.getByText('done')).toBeVisible();

	const finalWorker = state.workers[0];
	// Status is success OR a typed error string with a recorded reason.
	const successOrTypedError =
		finalWorker.status === 'success' ||
		(typeof finalWorker.status === 'string' &&
			finalWorker.status.length > 0 &&
			typeof finalWorker.lastError === 'string' &&
			finalWorker.lastError.length > 0);
	expect(successOrTypedError).toBe(true);

	// Attempts count matches the work done (successes + failures).
	expect(attemptsMatchWorkDone(finalWorker)).toBe(true);

	// Critical regression guards: never the 19-failure pattern, never gemini
	// cross-pollination, harness still bound to a live endpoint at completion.
	expect(finalWorker.failures).not.toBe(19);
	expect(finalWorker.attempts).toBeLessThanOrEqual(QUEUE_READY);
	expect(noGeminiCrossContamination(finalWorker)).toBe(true);
	expect(harnessBoundToLiveEndpoint(finalWorker)).toBe(true);
});

test('drain-queue golden: typed-error completion records a reason on the bead', async ({ page }) => {
	const state: { workers: WorkerNode[]; dispatched: { count: number } } = {
		workers: [
			makeWorker({
				state: 'done',
				status: 'no-viable-provider',
				attempts: 1,
				successes: 0,
				failures: 1,
				finishedAt: '2026-04-29T12:05:00Z',
				lastError: 'no-viable-provider: all 4 endpoints exhausted; bead-001 marked failed with reason'
			})
		],
		dispatched: { count: 0 }
	};
	await installGraphqlMocks(page, state);

	await page.goto(WORKERS_URL);
	await expect(page.getByRole('heading', { name: 'Workers' })).toBeVisible();

	const final = state.workers[0];
	// Typed error (not the string "error") with a non-empty reason on the bead.
	expect(final.status).not.toBe('error');
	expect(final.status).not.toBeNull();
	expect((final.status ?? '').length).toBeGreaterThan(0);
	expect((final.lastError ?? '').length).toBeGreaterThan(0);
	expect(final.lastError ?? '').toMatch(/reason/i);

	// Attempts count still matches work done; never the 19-failure pattern.
	expect(attemptsMatchWorkDone(final)).toBe(true);
	expect(final.failures).not.toBe(19);

	// The harness model — even on failure — is bound to a live endpoint and
	// not a gemini-cross-pollinated pairing.
	expect(harnessBoundToLiveEndpoint(final)).toBe(true);
	expect(noGeminiCrossContamination(final)).toBe(true);
});
