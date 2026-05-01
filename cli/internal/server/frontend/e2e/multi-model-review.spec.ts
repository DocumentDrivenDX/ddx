// Quorum review (multi-model review) end-to-end UI flow.
//
// Driving spec for the quorum-review UI surfaced from
// `ddx agent run --quorum=majority --harnesses=a,b,c`. The flow under test:
//
//   1. Operator opens a bead (or artifact) and dispatches a quorum review,
//      picking ≥2 harnesses and a verdict policy (majority / unanimous).
//   2. Server fans out to each harness and streams per-model findings.
//   3. UI shows a per-arm column with verdict + structured findings, plus a
//      rollup verdict computed from the policy.
//   4. Operator can drill into a single arm's raw output and link back to the
//      originating bead / artifact.
//
// The quorum-review view is not yet implemented (PRD §437 "Multi-agent review
// workflow produces structured findings from quorum"). These tests are checked
// in as a TDD seed: every case is marked `test.fixme` so CI stays green; once
// the view ships, drop the fixme markers one case at a time as features land.
// Mocks below describe the GraphQL contract the view is expected to consume.

import { expect, test, type Page } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const BEAD_ID = 'ddx-quorum-001';

const REVIEW_BASE_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/beads/${BEAD_ID}/reviews/quorum`;

const HARNESSES = [
	{ name: 'claude', provider: 'anthropic', model: 'claude-opus-4-7' },
	{ name: 'codex', provider: 'openai', model: 'gpt-5.4' },
	{ name: 'gemini', provider: 'google', model: 'gemini-2.5-pro' }
];

const REVIEW_ID = 'qrv-001';

const QUORUM_REVIEW = {
	id: REVIEW_ID,
	beadId: BEAD_ID,
	policy: 'majority',
	state: 'completed',
	rollupVerdict: 'PASS',
	createdAt: '2026-05-01T07:00:00Z',
	completedAt: '2026-05-01T07:04:13Z',
	arms: [
		{
			harness: 'claude',
			provider: 'anthropic',
			model: 'claude-opus-4-7',
			verdict: 'PASS',
			summary: 'Acceptance criteria satisfied; no blocking issues.',
			findings: [
				{ severity: 'info', message: 'Consider extracting the retry helper.' }
			],
			rawOutputUrl: `/api/quorum-reviews/${REVIEW_ID}/arms/claude/raw`
		},
		{
			harness: 'codex',
			provider: 'openai',
			model: 'gpt-5.4',
			verdict: 'PASS',
			summary: 'Looks good. One nit on naming.',
			findings: [
				{ severity: 'nit', message: 'Rename `tmp` to `pendingMerge` for clarity.' }
			],
			rawOutputUrl: `/api/quorum-reviews/${REVIEW_ID}/arms/codex/raw`
		},
		{
			harness: 'gemini',
			provider: 'google',
			model: 'gemini-2.5-pro',
			verdict: 'BLOCK',
			summary: 'Found a missing nil-check on the merge path.',
			findings: [
				{
					severity: 'blocking',
					message: 'cli/internal/agent/merge.go:142 dereferences result without nil-check.'
				}
			],
			rawOutputUrl: `/api/quorum-reviews/${REVIEW_ID}/arms/gemini/raw`
		}
	]
};

async function mockQuorumReview(
	page: Page,
	opts: {
		dispatchFn?: (req: Record<string, unknown>) => Record<string, unknown>;
		review?: typeof QUORUM_REVIEW;
	} = {}
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
			return;
		}
		if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
			return;
		}
		if (body.query.includes('Harnesses') || body.query.includes('harnesses')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { harnesses: HARNESSES } })
			});
			return;
		}
		if (body.query.includes('QuorumReview') || body.query.includes('quorumReview')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { quorumReview: opts.review ?? QUORUM_REVIEW } })
			});
			return;
		}
		if (
			body.query.includes('QuorumReviewDispatch') ||
			body.query.includes('quorumReviewDispatch')
		) {
			const result = opts.dispatchFn
				? opts.dispatchFn(body.variables ?? {})
				: { id: REVIEW_ID, state: 'queued' };
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { quorumReviewDispatch: result } })
			});
			return;
		}
		await route.continue();
	});
}

test.describe('quorum review UI', () => {
	test('dispatch: bead page lets operator pick harnesses and verdict policy', async ({
		page
	}) => {
		test.fixme(true, 'quorum review UI not implemented yet (PRD §437)');

		let dispatched: Record<string, unknown> | null = null;
		await mockQuorumReview(page, {
			dispatchFn: (req) => {
				dispatched = req;
				return { id: REVIEW_ID, state: 'queued', armCount: 2 };
			}
		});

		await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/beads/${BEAD_ID}`);

		await page.getByRole('button', { name: /quorum review/i }).click();

		const dialog = page.getByRole('dialog', { name: /quorum review/i });
		await expect(dialog).toBeVisible();

		// Harness checkboxes pre-populated from /graphql harnesses query.
		await dialog.getByRole('checkbox', { name: /claude/i }).check();
		await dialog.getByRole('checkbox', { name: /codex/i }).check();

		// Policy selector defaults to majority.
		const policy = dialog.getByRole('combobox', { name: /policy|verdict/i });
		await expect(policy).toHaveValue('majority');
		await policy.selectOption('unanimous');

		await dialog.getByRole('button', { name: /start|dispatch/i }).click();

		await expect.poll(() => dispatched).not.toBeNull();
		expect(dispatched).toMatchObject({
			beadId: BEAD_ID,
			policy: 'unanimous',
			harnesses: expect.arrayContaining(['claude', 'codex'])
		});

		// Operator is taken to the review detail page on dispatch.
		await expect(page).toHaveURL(new RegExp(`/reviews/quorum/${REVIEW_ID}$`));
	});

	test('view: per-arm columns render verdicts, summaries, and findings', async ({ page }) => {
		test.fixme(true, 'quorum review UI not implemented yet (PRD §437)');

		await mockQuorumReview(page);
		await page.goto(`${REVIEW_BASE_URL}/${REVIEW_ID}`);

		// Three arm columns, one per harness.
		const arms = page.getByTestId('quorum-arm');
		await expect(arms).toHaveCount(3);

		for (const arm of QUORUM_REVIEW.arms) {
			const col = page.getByTestId(`quorum-arm-${arm.harness}`);
			await expect(col).toBeVisible();
			await expect(col).toContainText(arm.model);
			await expect(col.getByTestId('arm-verdict')).toContainText(arm.verdict);
			await expect(col).toContainText(arm.summary);
			for (const finding of arm.findings) {
				await expect(col).toContainText(finding.message);
			}
		}
	});

	test('rollup: majority policy produces PASS when 2/3 arms pass', async ({ page }) => {
		test.fixme(true, 'quorum review UI not implemented yet (PRD §437)');

		await mockQuorumReview(page);
		await page.goto(`${REVIEW_BASE_URL}/${REVIEW_ID}`);

		const rollup = page.getByTestId('quorum-rollup-verdict');
		await expect(rollup).toBeVisible();
		await expect(rollup).toContainText(/PASS/);
		await expect(rollup).toContainText(/majority/i);
		await expect(rollup).toContainText(/2\s*\/\s*3/);
	});

	test('rollup: unanimous policy with one BLOCK arm produces BLOCK', async ({ page }) => {
		test.fixme(true, 'quorum review UI not implemented yet (PRD §437)');

		await mockQuorumReview(page, {
			review: { ...QUORUM_REVIEW, policy: 'unanimous', rollupVerdict: 'BLOCK' }
		});
		await page.goto(`${REVIEW_BASE_URL}/${REVIEW_ID}`);

		const rollup = page.getByTestId('quorum-rollup-verdict');
		await expect(rollup).toContainText(/BLOCK/);
		await expect(rollup).toContainText(/unanimous/i);
	});

	test('drill-in: arm column links to raw output and back to bead', async ({ page }) => {
		test.fixme(true, 'quorum review UI not implemented yet (PRD §437)');

		await mockQuorumReview(page);
		await page.goto(`${REVIEW_BASE_URL}/${REVIEW_ID}`);

		const geminiCol = page.getByTestId('quorum-arm-gemini');
		const rawLink = geminiCol.getByRole('link', { name: /raw output/i });
		await expect(rawLink).toHaveAttribute(
			'href',
			`/api/quorum-reviews/${REVIEW_ID}/arms/gemini/raw`
		);

		await page.getByRole('link', { name: new RegExp(BEAD_ID) }).click();
		await expect(page).toHaveURL(
			new RegExp(`/projects/${PROJECT_ID}/beads/${BEAD_ID}$`)
		);
	});

	test('streaming: in-flight review shows pending arms before completion', async ({ page }) => {
		test.fixme(true, 'quorum review UI not implemented yet (PRD §437)');

		const inflight = {
			...QUORUM_REVIEW,
			state: 'running',
			rollupVerdict: null,
			completedAt: null,
			arms: [
				{ ...QUORUM_REVIEW.arms[0] },
				{
					harness: 'codex',
					provider: 'openai',
					model: 'gpt-5.4',
					verdict: null,
					summary: null,
					findings: [],
					rawOutputUrl: null
				},
				{
					harness: 'gemini',
					provider: 'google',
					model: 'gemini-2.5-pro',
					verdict: null,
					summary: null,
					findings: [],
					rawOutputUrl: null
				}
			]
		};

		await mockQuorumReview(page, { review: inflight as typeof QUORUM_REVIEW });
		await page.goto(`${REVIEW_BASE_URL}/${REVIEW_ID}`);

		await expect(page.getByTestId('quorum-rollup-verdict')).toContainText(/running|pending/i);
		await expect(page.getByTestId('quorum-arm-codex').getByTestId('arm-verdict')).toContainText(
			/pending|running|—/
		);
		await expect(page.getByTestId('quorum-arm-claude').getByTestId('arm-verdict')).toContainText(
			'PASS'
		);
	});
});
