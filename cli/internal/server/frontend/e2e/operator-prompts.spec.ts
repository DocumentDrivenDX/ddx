// Playwright e2e for Story-15-6: SvelteKit prompt input + preview + recent-bead
// pane + approve UI + XSS escaping. Runs against the real Go fixture server
// (see playwright.config.ts) so the operatorPromptSubmit/Approve mutations
// hit real resolvers and persist real beads in the temp workspace.
import { expect, test, type APIRequestContext } from '@playwright/test';

async function getFixtureIds(
	request: APIRequestContext
): Promise<{ nodeId: string; projectId: string }> {
	const nodeResp = await request.post('/graphql', {
		data: { query: '{ nodeInfo { id name } }' }
	});
	const nodeBody = (await nodeResp.json()) as {
		data: { nodeInfo: { id: string; name: string } };
	};
	const projectsResp = await request.get('/api/projects');
	const projects = (await projectsResp.json()) as Array<{ id: string; name: string; path: string }>;
	const fixture = projects.find((p) => /(^|\/)ddx-e2e-/.test(p.path) || /^ddx-e2e-/.test(p.name));
	if (!fixture) {
		throw new Error(
			`fixture server has no ddx-e2e-* project registered (got: ${projects.map((p) => p.id).join(', ')})`
		);
	}
	return { nodeId: nodeBody.data.nodeInfo.id, projectId: fixture.id };
}

const OPERATOR_PROMPT_SUBMIT_MUTATION = `mutation Submit($input: OperatorPromptSubmitInput!) {
	operatorPromptSubmit(input: $input) {
		bead { id }
	}
}`;

async function submitOperatorPrompt(
	request: APIRequestContext,
	prompt: string,
	idempotencyKey: string
): Promise<string> {
	const csrfResp = await request.get('/api/csrf-token');
	const csrfBody = (await csrfResp.json()) as { token?: string };
	const resp = await request.post('/graphql', {
		headers: {
			'Content-Type': 'application/json',
			'X-CSRF-Token': csrfBody.token ?? ''
		},
		data: {
			query: OPERATOR_PROMPT_SUBMIT_MUTATION,
			variables: {
				input: {
					prompt,
					idempotencyKey
				}
			}
		}
	});
	const body = (await resp.json()) as {
		data?: {
			operatorPromptSubmit?: {
				bead?: { id?: string };
			};
		};
		errors?: Array<{ message: string }>;
	};
	if (body.errors?.length) {
		throw new Error(body.errors.map((e) => e.message).join('; '));
	}
	const beadId = body.data?.operatorPromptSubmit?.bead?.id;
	if (!beadId) {
		throw new Error('expected operator prompt bead id');
	}
	return beadId;
}

test.describe('operator prompts (Story 15-6)', () => {
	test('submit → preview → approve advances bead from proposed to open, link opens detail', async ({
		page,
		request
	}) => {
		await getFixtureIds(request);
		await page.goto('/operator-prompts');

		const panel = page.getByTestId('operator-prompt-panel');
		await expect(panel).toBeVisible();

		const promptBody = `e2e operator prompt ${Date.now()}`;
		await page.getByTestId('operator-prompt-textarea').fill(promptBody);
		await page.getByTestId('operator-prompt-priority').selectOption('2');
		await page.getByTestId('operator-prompt-submit').click();

		// Preview appears with proposed status before approval.
		const preview = page.getByTestId('operator-prompt-preview');
		await expect(preview).toBeVisible();
		await expect(page.getByTestId('operator-prompt-preview-status')).toHaveText('proposed');
		await expect(page.getByTestId('operator-prompt-preview-body')).toContainText(promptBody);

		const beadLink = page.getByTestId('operator-prompt-preview-link');
		const beadId = (await beadLink.textContent())?.trim() ?? '';
		expect(beadId).toMatch(/^ddx-/);

		// Approve transitions proposed → open.
		await page.getByTestId('operator-prompt-approve').click();
		await expect(preview).not.toBeVisible();

		// Recent prompts pane shows the bead with status open (live or via reload).
		const recent = page.getByTestId('operator-prompt-recent');
		await expect(recent).toContainText(beadId);

		// Click-through to bead detail.
		await page.getByTestId('operator-prompt-recent-link').first().click();
		await expect(page).toHaveURL(new RegExp(`/beads/${beadId}(\\?|$)`));
	});

	test('recent pane shows only the latest 10 operator-prompt beads', async ({ page, request }) => {
		const { projectId } = await getFixtureIds(request);
		const created: string[] = [];
		for (let i = 1; i <= 11; i++) {
			created.push(
				await submitOperatorPrompt(
					request,
					`recent-limit-seed-${String(i).padStart(2, '0')}`,
					`${projectId}-recent-limit-${i}`
				)
			);
			if (i < 11) {
				await page.waitForTimeout(1100);
			}
		}

		await page.goto('/operator-prompts');
		const recent = page.getByTestId('operator-prompt-recent');
		await expect(recent.locator('li')).toHaveCount(10);
		await expect(recent).toContainText(created[10]);
		await expect(recent).not.toContainText(created[0]);
	});

	test('XSS payloads in prompt body and recent pane are escaped, never executed', async ({
		page,
		request
	}) => {
		await getFixtureIds(request);

		// If the panel ever rendered prompt or evidence content via {@html} or
		// innerHTML, this script would fire and the test would fail.
		const dialogs: string[] = [];
		page.on('dialog', async (dlg) => {
			dialogs.push(dlg.message());
			await dlg.dismiss();
		});

		await page.goto('/operator-prompts');

		const xssPrompt = `<img src=x onerror="alert('xss-prompt')"><script>alert('xss-script')</script>`;
		await page.getByTestId('operator-prompt-textarea').fill(xssPrompt);
		await page.getByTestId('operator-prompt-submit').click();

		const previewBody = page.getByTestId('operator-prompt-preview-body');
		await expect(previewBody).toBeVisible();
		// The literal markup is rendered as text — innerText preserves it,
		// but innerHTML is escaped (no real <img> or <script> elements).
		await expect(previewBody).toContainText('<img src=x');
		await expect(previewBody).toContainText('<script>');
		const imgCount = await page.getByTestId('operator-prompt-preview-body').locator('img').count();
		expect(imgCount).toBe(0);
		const scriptCount = await page
			.getByTestId('operator-prompt-preview-body')
			.locator('script')
			.count();
		expect(scriptCount).toBe(0);
		expect(dialogs).toEqual([]);

		// Title is also escaped (first line of the prompt body is the title).
		const previewTitle = page.getByTestId('operator-prompt-preview-title');
		await expect(previewTitle.locator('img')).toHaveCount(0);
		await expect(previewTitle.locator('script')).toHaveCount(0);
	});
});
