import { test, expect } from '@playwright/test';

// DDx Server UI — visual regression screenshots
// These capture each page for visual review and regression detection.
// Run: bunx playwright test e2e/screenshots.spec.ts --update-snapshots
// to update baselines after intentional changes.
//
// Dark/light parity is a FEAT-008 frontend-design gate
// (docs/helix/01-frame/concerns.md#frontend-design). Every page is
// snapshotted in both modes; any theme-specific palette drift fails CI.

const PAGES = [
	{ path: '/', name: 'dashboard', ready: 'h1', maskStarted: true, tolerance: 0.02 },
	{ path: '/beads', name: 'beads-kanban', ready: 'text=OPEN', tolerance: 0.04 },
	{ path: '/documents', name: 'documents', ready: 'h1', tolerance: 0.02 },
	{ path: '/graph', name: 'graph', ready: 'h1', tolerance: 0.06 },
	{ path: '/agent', name: 'agent', ready: 'h1', tolerance: 0.04 },
	{
		path: '/nodes/local-node/projects/local-project',
		name: 'project-overview',
		ready: 'text=Actions',
		tolerance: 0.04
	},
	{ path: '/personas', name: 'personas', ready: 'text=Personas', tolerance: 0.04 },
	{
		path: '/nodes/local-node/projects/local-project/efficacy',
		name: 'efficacy',
		ready: 'text=Efficacy',
		tolerance: 0.04
	}
] as const;

const MODES = ['light', 'dark'] as const;

test.describe('DDx Server UI Screenshots', () => {
	for (const mode of MODES) {
		for (const pg of PAGES) {
			test(`${pg.name} (${mode})`, async ({ page }) => {
				await page.addInitScript((m) => {
					window.localStorage.setItem('mode-watcher-mode', m);
				}, mode);

				await page.goto(pg.path);
				await page.waitForSelector(pg.ready);
				await page.waitForTimeout(500);

				await expect(page).toHaveScreenshot(`${pg.name}-${mode}.png`, {
					fullPage: true,
					maxDiffPixelRatio: pg.tolerance,
					mask: pg.maskStarted ? [page.locator('text=/^Started:/')] : undefined
				});
			});
		}
	}
});
