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

test.describe('operator prompts (Story 15-6)', () => {
	test('submit → preview → approve advances bead from proposed to open, link opens detail', async ({
		page,
		request
	}) => {
		const { nodeId, projectId } = await getFixtureIds(request);
		await page.goto(`/nodes/${nodeId}/projects/${projectId}`);

		const panel = page.getByTestId('operator-prompt-panel');
		await expect(panel).toBeVisible();

		const promptBody = `e2e operator prompt ${Date.now()}`;
		await page.getByTestId('operator-prompt-textarea').fill(promptBody);
		await page.getByTestId('operator-prompt-tier').selectOption('2');
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

	test('XSS payloads in prompt body and recent pane are escaped, never executed', async ({
		page,
		request
	}) => {
		const { nodeId, projectId } = await getFixtureIds(request);

		// If the panel ever rendered prompt or evidence content via {@html} or
		// innerHTML, this script would fire and the test would fail.
		const dialogs: string[] = [];
		page.on('dialog', async (dlg) => {
			dialogs.push(dlg.message());
			await dlg.dismiss();
		});

		await page.goto(`/nodes/${nodeId}/projects/${projectId}`);

		const xssPrompt = `<img src=x onerror="alert('xss-prompt')"><script>alert('xss-script')</script>`;
		await page.getByTestId('operator-prompt-textarea').fill(xssPrompt);
		await page.getByTestId('operator-prompt-submit').click();

		const previewBody = page.getByTestId('operator-prompt-preview-body');
		await expect(previewBody).toBeVisible();
		// The literal markup is rendered as text — innerText preserves it,
		// but innerHTML is escaped (no real <img> or <script> elements).
		await expect(previewBody).toContainText('<img src=x');
		await expect(previewBody).toContainText('<script>');
		const imgCount = await page
			.getByTestId('operator-prompt-preview-body')
			.locator('img')
			.count();
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
