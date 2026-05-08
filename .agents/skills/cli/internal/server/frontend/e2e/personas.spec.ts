// FEAT-008 US-097: Developer Browses and Binds Personas
//
// These tests MUST FAIL until the Personas page lists installed personas,
// renders a detail view, and writes `persona_bindings` to the project's
// .ddx/config.yaml via a GraphQL mutation.

import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/personas`;

const PERSONAS = [
	{
		name: 'code-reviewer',
		roles: ['code-reviewer'],
		description: 'Strict reviewer focused on correctness + safety',
		body: '# Code Reviewer\n\nYou are a strict reviewer...',
		source: 'ddx-default-plugin',
		bindings: [{ projectId: 'proj-2', role: 'code-reviewer' }]
	},
	{
		name: 'test-engineer',
		roles: ['test-engineer', 'implementer'],
		description: 'Writes failing tests first; refuses to mock internals',
		body: '# Test Engineer\n\nYou write tests before code...',
		source: 'ddx-default-plugin',
		bindings: []
	}
];

async function mockPersonas(
	page: import('@playwright/test').Page,
	opts: { bindFn?: (req: Record<string, unknown>) => Record<string, unknown> | Error; existingBinding?: string } = {}
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
		} else if (body.query.includes('Personas') && !body.query.includes('PersonaBind')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { personas: PERSONAS } }) });
		} else if (body.query.includes('ProjectBindings')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						projectBindings: opts.existingBinding
							? { 'code-reviewer': opts.existingBinding }
							: {}
					}
				})
			});
		} else if (body.query.includes('PersonaBind') || body.query.includes('personaBind')) {
			const result = opts.bindFn
				? opts.bindFn(body.variables ?? {})
				: { ok: true, role: 'code-reviewer', persona: 'code-reviewer' };
			if (result instanceof Error) {
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ errors: [{ message: result.message }] }) });
				return;
			}
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { personaBind: result } }) });
		} else {
			await route.continue();
		}
	});
}

test('US-097.a: personas page renders a card per installed persona', async ({ page }) => {
	await mockPersonas(page);
	await page.goto(BASE_URL);

	for (const p of PERSONAS) {
		const card = page.getByRole('article', { name: new RegExp(p.name, 'i') });
		await expect(card).toBeVisible();
		await expect(card).toContainText(p.description);
		for (const role of p.roles) {
			await expect(card.getByText(role, { exact: false })).toBeVisible();
		}
	}
});

test('US-097.b: clicking a card opens detail view with markdown body + current bindings', async ({ page }) => {
	await mockPersonas(page);
	await page.goto(BASE_URL);

	await page.getByRole('article', { name: /code-reviewer/i }).click();

	await expect(page).toHaveURL(/\/personas\/code-reviewer/);
	await expect(page.getByRole('heading', { level: 1, name: /code reviewer/i })).toBeVisible();
	await expect(page.getByText('You are a strict reviewer', { exact: false })).toBeVisible();

	// Bindings list.
	const bindingsRegion = page.getByRole('region', { name: /bindings/i });
	await expect(bindingsRegion).toBeVisible();
	await expect(bindingsRegion).toContainText(/proj-2/);
});

test('US-097.c: Bind to role form writes persona_bindings via mutation', async ({ page }) => {
	let captured: Record<string, unknown> | null = null;
	await mockPersonas(page, {
		bindFn: (req) => {
			captured = req;
			return { ok: true, role: req.role, persona: req.persona };
		}
	});

	await page.goto(`${BASE_URL}/code-reviewer`);
	await page.getByRole('button', { name: /bind to role/i }).click();

	const form = page.getByRole('dialog', { name: /bind/i });
	await form.getByRole('combobox', { name: /role/i }).selectOption('code-reviewer');
	await form.getByRole('combobox', { name: /project/i }).selectOption(PROJECT_ID);
	await form.getByRole('button', { name: /bind|save|submit/i }).click();

	await expect.poll(() => captured).not.toBeNull();
	expect(captured).toMatchObject({
		role: 'code-reviewer',
		persona: 'code-reviewer',
		projectId: PROJECT_ID
	});

	await expect(page.getByRole('status')).toContainText(/bound|saved/i);
});

test('US-097.d: overwriting an existing binding requires confirmation', async ({ page }) => {
	let captured: Record<string, unknown> | null = null;
	await mockPersonas(page, {
		existingBinding: 'test-engineer',
		bindFn: (req) => {
			captured = req;
			return { ok: true, role: req.role, persona: req.persona };
		}
	});

	await page.goto(`${BASE_URL}/code-reviewer`);
	await page.getByRole('button', { name: /bind to role/i }).click();

	const form = page.getByRole('dialog', { name: /bind/i });
	await form.getByRole('combobox', { name: /role/i }).selectOption('code-reviewer');
	await form.getByRole('combobox', { name: /project/i }).selectOption(PROJECT_ID);
	await form.getByRole('button', { name: /bind|save|submit/i }).click();

	// Warning must name the existing persona.
	const warning = form.getByRole('alert');
	await expect(warning).toBeVisible();
	await expect(warning).toContainText(/replace/i);
	await expect(warning).toContainText(/test-engineer/);

	// No mutation fired yet.
	expect(captured).toBeNull();

	await form.getByRole('button', { name: /confirm|overwrite/i }).click();
	await expect.poll(() => captured).not.toBeNull();
});
