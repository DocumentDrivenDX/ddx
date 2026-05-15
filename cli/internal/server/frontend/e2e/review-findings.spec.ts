import { expect, test, type Page } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT = { id: 'proj-1', name: 'Project Alpha', path: '/repos/alpha' };
const ARTIFACT = {
	id: 'artifact-001',
	path: 'docs/spec.md',
	title: 'Artifact Under Review',
	mediaType: 'text/markdown',
	sha256: 'sha-001',
	staleness: 'fresh',
	description: 'Review target',
	updatedAt: '2026-05-15T20:00:00Z',
	ddxFrontmatter: null,
	content: '# Artifact\n\nReview body',
	typeDefinitions: [],
	generatedBy: null
};
const REVIEW_BASE_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT.id}/artifacts/${ARTIFACT.id}`;
const FINDINGS = [
	'Missing nil-check on merge path.',
	'Add regression test for cancelled-session recovery.'
];

type GraphqlVariables = Record<string, unknown>;

async function mockReviewFindingsPage(
	page: Page,
	opts: {
		operatorPromptAvailable: boolean;
		onOperatorPromptSubmit?: (variables: GraphqlVariables) => void;
		onBeadCreate?: (variables: GraphqlVariables) => void;
	}
) {
	await page.route('/api/csrf-token', async (route) => {
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({ token: 'csrf-test-token' })
		});
	});

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: GraphqlVariables;
		};

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}

		if (body.query.includes('ProjectsForLayout')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: [{ node: PROJECT }] } }
				})
			});
			return;
		}

		if (body.query.includes('ArtifactDetail')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { artifact: ARTIFACT } })
			});
			return;
		}

		if (
			body.query.includes('ReviewPanelMutationCapabilities') ||
			body.query.includes('__type(name: "Mutation")')
		) {
			const fields = opts.operatorPromptAvailable
				? [{ name: 'beadCreate' }, { name: 'operatorPromptSubmit' }]
				: [{ name: 'beadCreate' }];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { __type: { fields } } })
			});
			return;
		}

		if (body.query.includes('ReviewSessionStart')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						reviewSessionStart: {
							id: 'rev-001',
							artifactId: ARTIFACT.id,
							artifactSha: ARTIFACT.sha256,
							status: 'active',
							costUSD: 0,
							maxBillableUSD: 0,
							turns: []
						}
					}
				})
			});
			return;
		}

		if (body.query.includes('ReviewSessionRespond')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						reviewSessionRespond: {
							id: 'rev-001',
							artifactId: ARTIFACT.id,
							artifactSha: ARTIFACT.sha256,
							status: 'completed',
							costUSD: 0,
							maxBillableUSD: 0,
							turns: [
								{
									actor: 'user',
									content: 'Check for bugs, regressions, and missing tests.',
									costUSD: 0,
									createdAt: '2026-05-15T20:01:00Z'
								},
								{
									actor: 'reviewer',
									content: FINDINGS.map((finding) => `- ${finding}`).join('\n'),
									costUSD: 0,
									createdAt: '2026-05-15T20:01:01Z'
								}
							]
						}
					}
				})
			});
			return;
		}

		if (
			body.query.includes('OperatorPromptSubmit') ||
			body.query.includes('operatorPromptSubmit(')
		) {
			opts.onOperatorPromptSubmit?.(body.variables ?? {});
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						operatorPromptSubmit: {
							deduplicated: false,
							autoApproved: false,
							bead: {
								id: 'ddx-op-001',
								title: 'Address review finding for Artifact Under Review',
								status: 'proposed',
								priority: 2,
								issueType: 'task',
								createdAt: '2026-05-15T20:02:00Z',
								updatedAt: '2026-05-15T20:02:00Z',
								labels: ['kind:operator-prompt'],
								description: null
							}
						}
					}
				})
			});
			return;
		}

		if (body.query.includes('ReviewFindingBeadCreate') || body.query.includes('beadCreate(')) {
			opts.onBeadCreate?.(body.variables ?? {});
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						beadCreate: {
							id: 'ddx-follow-001',
							title: 'Follow up review finding: Missing nil-check on merge path.',
							status: 'open',
							priority: 2,
							issueType: 'task',
							createdAt: '2026-05-15T20:02:00Z',
							updatedAt: '2026-05-15T20:02:00Z',
							labels: null,
							description: null
						}
					}
				})
			});
			return;
		}

		await route.continue();
	});
}

test('review findings submit through operatorPromptSubmit when Story 15 mutations are available', async ({
	page
}) => {
	let submitted: GraphqlVariables | null = null;
	await mockReviewFindingsPage(page, {
		operatorPromptAvailable: true,
		onOperatorPromptSubmit: (variables) => {
			submitted = variables;
		}
	});

	await page.goto(REVIEW_BASE_URL);
	await page.getByTestId('review-input').fill('Check for bugs, regressions, and missing tests.');
	await page.getByTestId('review-submit').click();

	await expect(page.getByTestId('review-findings')).toBeVisible();
	await expect(page.getByTestId('review-finding')).toHaveCount(2);
	await expect(page.getByTestId('review-finding-action').first()).toHaveText(/use as edit prompt/i);

	await page.getByTestId('review-finding-action').first().click();

	await expect.poll(() => submitted).not.toBeNull();
	expect(submitted).toMatchObject({
		input: {
			priority: 2,
			autoApprove: false
		}
	});
	expect(String(submitted?.input?.prompt ?? '')).toContain(FINDINGS[0]);

	await expect(page.getByTestId('review-finding-result')).toContainText(
		'Created operator-prompt bead'
	);
	await expect(page.getByTestId('review-finding-link')).toHaveText('ddx-op-001');
});

test('review findings fall back to beadCreate when Story 15 mutations are unavailable', async ({
	page
}) => {
	let created: GraphqlVariables | null = null;
	await mockReviewFindingsPage(page, {
		operatorPromptAvailable: false,
		onBeadCreate: (variables) => {
			created = variables;
		}
	});

	await page.goto(REVIEW_BASE_URL);
	await page.getByTestId('review-input').fill('Check for bugs, regressions, and missing tests.');
	await page.getByTestId('review-submit').click();

	await expect(page.getByTestId('review-findings')).toBeVisible();
	await expect(page.getByTestId('review-finding-action').first()).toHaveText(
		/create follow-up bead/i
	);

	await page.getByTestId('review-finding-action').first().click();

	await expect.poll(() => created).not.toBeNull();
	expect(created).toMatchObject({
		input: {
			status: 'open',
			priority: 2,
			issueType: 'task'
		}
	});
	expect(String(created?.input?.description ?? '')).toContain(FINDINGS[0]);

	await expect(page.getByTestId('review-finding-result')).toContainText('Created follow-up bead');
	await expect(page.getByTestId('review-finding-link')).toHaveText('ddx-follow-001');
});
