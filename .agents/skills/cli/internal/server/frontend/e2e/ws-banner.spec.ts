import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECTS = [{ id: 'proj-1', name: 'Project Alpha', path: '/repos/alpha' }];

const EMPTY_BEADS = {
	beadsByProject: {
		edges: [],
		pageInfo: { hasNextPage: false, endCursor: null },
		totalCount: 0
	}
};

/** Mock HTTP /graphql so the app can boot and navigate. */
async function mockGraphQL(page: import('@playwright/test').Page) {
	await page.route('**/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
		} else if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: EMPTY_BEADS })
			});
		} else if (body.query.includes('Projects') || body.query.includes('projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('ProviderStatuses')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						providerStatuses: [],
						harnessStatuses: [],
						defaultRouteStatus: null
					}
				})
			});
		} else {
			await route.continue();
		}
	});
}

// ---------------------------------------------------------------------------
// TC: WebSocket disconnected banner
// ---------------------------------------------------------------------------
test.describe('WebSocket disconnected banner', () => {
	test('banner appears when WS drops and clears on reconnect', async ({ page }) => {
		await mockGraphQL(page);

		// Track WS routes so we can close/reconnect
		let closeWs: (() => void) | null = null;

		await page.routeWebSocket('**/graphql', (ws) => {
			closeWs = () => ws.close({ code: 1006, reason: 'test disconnect' });

			ws.onMessage((msg) => {
				const parsed = JSON.parse(msg as string);
				if (parsed.type === 'connection_init') {
					ws.send(JSON.stringify({ type: 'connection_ack' }));
				}
			});
		});

		// Navigate to a page that creates a subscription (beads page)
		await page.goto('/nodes/node-abc/projects/proj-1/beads');

		// Wait for the page to render the beads layout
		await expect(page.locator('h1:has-text("Beads")')).toBeVisible({ timeout: 5000 });

		// Wait for the WS connection to be established by the subscription effect
		await page.waitForTimeout(1000);

		const banner = page.getByTestId('ws-disconnected-banner');

		// Banner should NOT be visible when connected
		await expect(banner).not.toBeVisible({ timeout: 3000 });

		// Disconnect the WebSocket
		expect(closeWs).not.toBeNull();
		closeWs!();

		// AC-1: Banner should appear within ~1s
		await expect(banner).toBeVisible({ timeout: 2000 });
		await expect(banner).toContainText(/(disconnected|reconnecting)/);

		// Set up a new WS handler that will accept the reconnection
		await page.routeWebSocket('**/graphql', (ws) => {
			ws.onMessage((msg) => {
				const parsed = JSON.parse(msg as string);
				if (parsed.type === 'connection_init') {
					ws.send(JSON.stringify({ type: 'connection_ack' }));
				}
			});
		});

		// AC-2: Banner should clear on reconnect (graphql-ws auto-retries)
		await expect(banner).not.toBeVisible({ timeout: 10000 });
	});

	test('banner is hidden when WS has never connected', async ({ page }) => {
		await mockGraphQL(page);

		// Navigate to a page WITHOUT subscriptions
		await page.goto('/nodes/node-abc/providers');

		const banner = page.getByTestId('ws-disconnected-banner');

		// Should not appear — no subscription was ever attempted
		await expect(banner).not.toBeVisible({ timeout: 2000 });
	});
});
