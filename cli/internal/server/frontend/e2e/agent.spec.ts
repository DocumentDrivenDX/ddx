import { expect, test } from '@playwright/test';

test('agent route redirects directly to the run layer', async ({ page }) => {
	const navigations: string[] = [];
	page.on('framenavigated', (frame) => {
		if (frame !== page.mainFrame()) {
			return;
		}
		const url = new URL(frame.url());
		navigations.push(`${url.pathname}${url.search}`);
	});

	await page.goto('/agent');

	await expect(page).toHaveURL(/\/runs\?layer=run$/);
	const normalizedNavigations = navigations.filter(
		(url, index) => index === 0 || url !== navigations[index - 1]
	);
	expect(normalizedNavigations).toHaveLength(2);
	expect(normalizedNavigations[0]).toBe('/agent');
	expect(normalizedNavigations[1]).toMatch(/\/runs\?layer=run$/);
	await expect(page.getByRole('heading', { name: 'Runs' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'run', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
});
