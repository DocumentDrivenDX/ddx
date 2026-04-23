// FEAT-008 US-096: Operator Views Model Efficacy and Runs Comparisons
//
// These tests MUST FAIL until an Efficacy view exists. The view aggregates
// `kind:cost` + `kind:routing` evidence from closed beads, groups by
// (harness, provider, model), and lets the operator dispatch
// `ddx agent compare` A/B runs. Tests drive the implementation per TDD.

import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/efficacy`;

const EFFICACY_ROWS = [
	{
		harness: 'codex',
		provider: 'openai',
		model: 'gpt-5',
		attempts: 42,
		successes: 40,
		successRate: 0.9524,
		medianInputTokens: 3200,
		medianOutputTokens: 1100,
		medianDurationMs: 28000,
		medianCostUsd: 0.032,
		warning: null
	},
	{
		harness: 'claude',
		provider: 'anthropic',
		model: 'claude-sonnet-4-6',
		attempts: 60,
		successes: 57,
		successRate: 0.95,
		medianInputTokens: 4100,
		medianOutputTokens: 1500,
		medianDurationMs: 45000,
		medianCostUsd: 0.047,
		warning: null
	},
	{
		harness: 'codex',
		provider: 'vidar-omlx',
		model: 'qwen3.6-35b',
		attempts: 80,
		successes: 48,
		successRate: 0.6,
		medianInputTokens: 2800,
		medianOutputTokens: 900,
		medianDurationMs: 62000,
		medianCostUsd: null,
		warning: { kind: 'below-adaptive-floor', threshold: 0.7 }
	}
];

const ATTEMPTS_DETAIL = {
	rowKey: 'codex|openai|gpt-5',
	attempts: Array.from({ length: 10 }, (_, i) => ({
		beadId: `ddx-attempt-${i}`,
		outcome: i % 4 === 3 ? 'failed' : 'succeeded',
		durationMs: 20000 + i * 1500,
		costUsd: 0.02 + i * 0.002,
		evidenceBundleUrl: `/executions/exec-${i}/result.json`
	}))
};

// TEN_K_SESSION_FIXTURE_ROWS mirrors the aggregated shape the backend produces
// from the 10k-session fixture seeded by seedEfficacySessionFixture
// (cli/internal/server/graphql/efficacy_sessions_test.go). The backend
// groups 10k sessions across 10 (harness, provider, model) groups — keep
// these rows in lock-step with that fixture so the UI smoke test renders the
// same shape the production aggregation would. The real-backend counterpart
// is TestEfficacyRowsSmokeOverRealBackend in the same Go test file; together
// they cover AC §8 (UI rendering here, real HTTP round-trip there).
const TEN_K_SESSION_FIXTURE_GROUPS = [
	{ harness: 'agent', provider: 'openai', model: 'gpt-5.4' },
	{ harness: 'agent', provider: 'openai', model: 'gpt-5.4-mini' },
	{ harness: 'codex', provider: 'openai', model: 'gpt-5.3-codex' },
	{ harness: 'claude', provider: 'anthropic', model: 'claude-sonnet-4-6' },
	{ harness: 'claude', provider: 'anthropic', model: 'claude-opus-4-6' },
	{ harness: 'gemini', provider: 'google', model: 'gemini-2.5-pro' },
	{ harness: 'benchmark', provider: 'local', model: 'qwen3.5-27b' },
	{ harness: 'quorum', provider: 'openrouter', model: 'minimax/minimax-m2.7' },
	{ harness: 'agent-run', provider: 'moonshot', model: 'moonshot/kimi-k2.5' },
	{ harness: 'script', provider: 'vidar', model: 'qwen/qwen3-coder-next' }
] as const;

const TEN_K_SESSION_FIXTURE_ROWS = TEN_K_SESSION_FIXTURE_GROUPS.map((g, i) => ({
	rowKey: `${g.harness}|${g.provider}|${g.model}`,
	harness: g.harness,
	provider: g.provider,
	model: g.model,
	// 10 groups × ~1000 attempts per group ≈ 10k sessions; 10/11 succeed per
	// the seed loop's failure cadence.
	attempts: 1000,
	successes: 910,
	successRate: 0.91,
	medianInputTokens: 3500 + i,
	medianOutputTokens: 1200 + i,
	medianDurationMs: 5500 + i * 100,
	medianCostUsd: i % 5 === 0 ? null : 0.005 + i / 1000,
	warning: null
}));

async function mockEfficacy(
	page: import('@playwright/test').Page,
	opts: {
		compareFn?: (req: Record<string, unknown>) => Record<string, unknown>;
		rows?: typeof EFFICACY_ROWS;
	} = {}
) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('EfficacyRows') || body.query.includes('efficacyRows')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { efficacyRows: opts.rows ?? EFFICACY_ROWS } }) });
		} else if (body.query.includes('EfficacyAttempts') || body.query.includes('efficacyAttempts')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { efficacyAttempts: ATTEMPTS_DETAIL } }) });
		} else if (body.query.includes('ComparisonDispatch') || body.query.includes('comparisonDispatch')) {
			const result = opts.compareFn
				? opts.compareFn(body.variables ?? {})
				: { id: 'cmp-001', state: 'queued' };
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { comparisonDispatch: result } }) });
		} else if (body.query.includes('Comparisons')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { comparisons: [] } }) });
		} else {
			await route.continue();
		}
	});
}

test('US-096.a: efficacy table lists every (harness, provider, model) tuple with required columns', async ({ page }) => {
	await mockEfficacy(page);
	await page.goto(BASE_URL);

	const table = page.getByRole('table', { name: /efficacy/i });
	await expect(table).toBeVisible();

	// Column headers that MUST be present.
	for (const header of [/harness/i, /provider/i, /model/i, /attempts/i, /success/i, /tokens/i, /duration/i, /cost/i]) {
		await expect(table.getByRole('columnheader', { name: header })).toBeVisible();
	}

	// One row per tuple.
	for (const row of EFFICACY_ROWS) {
		const tr = table.getByRole('row', { name: new RegExp(`${row.harness}.*${row.model}`, 'i') });
		await expect(tr).toBeVisible();
		await expect(tr).toContainText(String(row.attempts));
	}

	// No-cost-signal case uses em-dash placeholder, not empty or "0".
	const noCostRow = table.getByRole('row', { name: /qwen3\.6/i });
	await expect(noCostRow).toContainText('—');
});

test('US-096.b: filtering by tier / label / spec-id updates table and encodes to URL', async ({ page }) => {
	await mockEfficacy(page);
	await page.goto(BASE_URL);

	await page.getByRole('combobox', { name: /tier/i }).selectOption('cheap');
	await expect(page).toHaveURL(/[?&]tier=cheap/);

	await page.getByRole('textbox', { name: /spec[- ]id/i }).fill('FEAT-008');
	await expect(page).toHaveURL(/[?&]spec-id=FEAT-008/);

	// Reload preserves filter state from URL.
	await page.reload();
	await expect(page.getByRole('combobox', { name: /tier/i })).toHaveValue('cheap');
	await expect(page.getByRole('textbox', { name: /spec[- ]id/i })).toHaveValue('FEAT-008');
});

test('US-096.c: Compare dispatches ddx agent compare and records appear under Comparisons', async ({ page }) => {
	let dispatched: Record<string, unknown> | null = null;
	await mockEfficacy(page, {
		compareFn: (req) => {
			dispatched = req;
			return { id: 'cmp-777', state: 'queued', armCount: 2 };
		}
	});

	await page.goto(BASE_URL);
	await page.getByRole('button', { name: /^compare$/i }).click();

	const dialog = page.getByRole('dialog', { name: /compare/i });
	await expect(dialog).toBeVisible();

	// Pick two (model, prompt) arms.
	await dialog.getByRole('button', { name: /add arm/i }).click();
	await dialog.getByRole('button', { name: /add arm/i }).click();

	const arms = dialog.getByTestId('comparison-arm');
	await expect(arms).toHaveCount(2);

	await arms.nth(0).getByRole('combobox', { name: /model/i }).selectOption('gpt-5');
	await arms.nth(0).getByRole('textbox', { name: /prompt/i }).fill('Summarize file X');
	await arms.nth(1).getByRole('combobox', { name: /model/i }).selectOption('claude-sonnet-4-6');
	await arms.nth(1).getByRole('textbox', { name: /prompt/i }).fill('Summarize file X');

	await dialog.getByRole('button', { name: /submit|start/i }).click();

	await expect.poll(() => dispatched).not.toBeNull();
	expect(dispatched).toMatchObject({
		arms: expect.arrayContaining([
			expect.objectContaining({ model: 'gpt-5' }),
			expect.objectContaining({ model: 'claude-sonnet-4-6' })
		])
	});

	await expect(page.getByRole('link', { name: /cmp-777/ })).toBeVisible();
});

test('US-096.d: success rate below adaptive floor shows warning badge with tooltip', async ({ page }) => {
	await mockEfficacy(page);
	await page.goto(BASE_URL);

	const qwenRow = page.getByRole('row', { name: /qwen3\.6/i });
	const badge = qwenRow.getByRole('img', { name: /warning|below.*floor/i });
	await expect(badge).toBeVisible();

	await badge.hover();
	const tooltip = page.getByRole('tooltip');
	await expect(tooltip).toContainText(/below.*(floor|threshold)/i);
	await expect(tooltip.getByRole('link', { name: /routing metrics/i })).toHaveAttribute('href', /routing/);
});

test('US-096.e: row click opens detail panel with last 10 attempts and evidence links', async ({ page }) => {
	await mockEfficacy(page);
	await page.goto(BASE_URL);

	await page.getByRole('row', { name: /gpt-5/i }).click();

	const panel = page.getByRole('complementary', { name: /attempts|detail/i });
	await expect(panel).toBeVisible();

	const rows = panel.getByRole('row');
	// header + 10 attempts
	await expect(rows).toHaveCount(11);

	for (let i = 0; i < 10; i++) {
		const bundleLink = panel.getByRole('link', { name: new RegExp(`attempt-${i}|exec-${i}`) });
		await expect(bundleLink).toBeVisible();
	}

	// Click-through to originating bead.
	await panel.getByRole('link', { name: /ddx-attempt-0/ }).click();
	await expect(page).toHaveURL(/\/beads\/ddx-attempt-0/);
});

test('smoke: efficacy opens with 10k-session rollup fixture and attempt details', async ({ page }) => {
	// AC §8 UI half. Renders the aggregated shape the 10k-session fixture
	// produces (see TEN_K_SESSION_FIXTURE_ROWS above) and exercises the
	// click-into EfficacyAttempts flow. The matching real-backend smoke —
	// TestEfficacyRowsSmokeOverRealBackend in
	// cli/internal/server/graphql/efficacy_sessions_test.go — drives the same
	// fixture through the real GraphQL HTTP handler.
	await mockEfficacy(page, { rows: TEN_K_SESSION_FIXTURE_ROWS });
	await page.goto(BASE_URL);

	const table = page.getByRole('table', { name: /efficacy/i });
	await expect(table).toBeVisible();
	// 10 data rows + 1 header row; the AC §8 floor is ≥5 rendered rows.
	const dataRows = table.getByRole('row').filter({ hasNot: page.getByRole('columnheader') });
	await expect(dataRows).toHaveCount(TEN_K_SESSION_FIXTURE_ROWS.length);
	expect(TEN_K_SESSION_FIXTURE_ROWS.length).toBeGreaterThanOrEqual(5);

	await table.getByRole('row', { name: /gpt-5\.4/i }).first().click();
	const panel = page.getByRole('complementary', { name: /attempts|detail/i });
	await expect(panel).toBeVisible();
	await expect(panel.getByRole('row')).toHaveCount(11);
});
