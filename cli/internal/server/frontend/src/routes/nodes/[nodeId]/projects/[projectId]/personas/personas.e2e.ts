import { expect, test, type Page } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };

const libraryPersona = {
	id: 'persona-architect',
	name: 'architect',
	roles: ['architect'],
	description: 'Library architect',
	tags: [],
	body: '# Architect\n',
	source: 'library',
	bindings: [],
	filePath: '/lib/architect.md',
	modTime: null
};

let projectPersonas: typeof libraryPersona[] = [];

type GqlBody = { query: string; variables?: Record<string, unknown> };

async function mockGraphQL(page: Page) {
	projectPersonas = [];
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
					data: {
						projects: {
							edges: [{ node: { id: 'proj-1', name: 'Project', path: '/tmp/proj' } }]
						}
					}
				})
			});
			return;
		}

		if (q.includes('query Personas')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { personas: [libraryPersona, ...projectPersonas] }
				})
			});
			return;
		}
		if (q.includes('ProjectBindings')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { projectBindings: '{}' } })
			});
			return;
		}
		if (q.includes('PersonaCreate')) {
			const vars = body.variables as { name: string };
			projectPersonas.push({
				...libraryPersona,
				id: `persona-${vars.name}`,
				name: vars.name,
				description: `Project persona ${vars.name}`,
				body: `# ${vars.name}\n`,
				source: 'project',
				filePath: `/proj/.ddx/personas/${vars.name}.md`
			});
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						personaCreate: {
							id: `persona-${vars.name}`,
							name: vars.name,
							source: 'project'
						}
					}
				})
			});
			return;
		}
		if (q.includes('PersonaUpdate')) {
			const vars = body.variables as { name: string };
			const p = projectPersonas.find((p) => p.name === vars.name);
			if (p) p.description = `Updated ${vars.name}`;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { personaUpdate: { id: `persona-${vars.name}`, name: vars.name, source: 'project' } }
				})
			});
			return;
		}
		if (q.includes('PersonaDelete')) {
			const vars = body.variables as { name: string };
			projectPersonas = projectPersonas.filter((p) => p.name !== vars.name);
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { personaDelete: { ok: true, name: vars.name } }
				})
			});
			return;
		}
		if (q.includes('PersonaFork')) {
			const vars = body.variables as { libraryName: string; newName: string | null };
			const target = vars.newName ?? vars.libraryName;
			projectPersonas.push({
				...libraryPersona,
				id: `persona-${target}`,
				name: target,
				description: `Forked from ${vars.libraryName}`,
				body: libraryPersona.body,
				source: 'project',
				filePath: `/proj/.ddx/personas/${target}.md`
			});
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { personaFork: { id: `persona-${target}`, name: target, source: 'project' } }
				})
			});
			return;
		}
		await route.continue();
	});
}

test('personas page shows explainer and source badges', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/nodes/node-abc/projects/proj-1/personas');

	const explainer = page.getByTestId('personas-explainer');
	await expect(explainer).toContainText('Personas are AI personality templates');
	await expect(explainer).toContainText('Library personas are shared');
	await expect(explainer).toContainText('project personas live with this project');

	await expect(page.getByTestId('persona-source-architect')).toHaveText('library');
});

test('empty project state shows hint', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/nodes/node-abc/projects/proj-1/personas');
	await expect(page.getByTestId('no-project-personas-hint')).toBeVisible();
});

test('create edit delete project persona', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/nodes/node-abc/projects/proj-1/personas');

	await page.getByTestId('persona-new-button').click();
	await page.getByTestId('persona-editor').waitFor();
	await page.getByTestId('persona-editor-name').fill('our-reviewer');
	// body already has a scaffold; save should succeed.
	await page.getByTestId('persona-editor-save').click();

	await expect(page.getByTestId('persona-source-our-reviewer')).toHaveText('project');

	// Edit.
	await page.getByTestId('persona-edit-our-reviewer').click();
	await page.getByTestId('persona-editor-body').fill(`---\nname: our-reviewer\nroles: [code-reviewer]\ndescription: Updated\ntags: []\n---\n\n# Updated\n`);
	await page.getByTestId('persona-editor-save').click();

	// Delete.
	await page.getByTestId('persona-delete-our-reviewer').click();
	await page.getByTestId('persona-delete-confirm').click();
	await expect(page.getByTestId('persona-source-our-reviewer')).toHaveCount(0);
});

test('fork library persona to project', async ({ page }) => {
	await mockGraphQL(page);
	page.on('dialog', (dialog) => dialog.accept('architect-local'));
	await page.goto('/nodes/node-abc/projects/proj-1/personas');

	await page.getByTestId('persona-fork-architect').click();
	await expect(page.getByTestId('persona-source-architect-local')).toHaveText('project');
});
