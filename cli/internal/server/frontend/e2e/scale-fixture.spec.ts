import { expect, test } from '@playwright/test';

async function getFixtureIds(
	request: import('@playwright/test').APIRequestContext
): Promise<{ nodeId: string; projectId: string }> {
	const nodeResp = await request.post('/graphql', {
		data: { query: '{ nodeInfo { id name } }' }
	});
	const nodeBody = (await nodeResp.json()) as {
		data: { nodeInfo: { id: string; name: string } };
	};
	const projectsResp = await request.get('/api/projects');
	const projects = (await projectsResp.json()) as Array<{
		id: string;
		name: string;
		path: string;
	}>;
	const fixture = projects.find((p) => /(^|\/)ddx-e2e-/.test(p.path) || /^ddx-e2e-/.test(p.name));
	if (!fixture) {
		throw new Error(
			`fixture server has no ddx-e2e-* project registered (got: ${projects
				.map((p) => p.id)
				.join(', ')})`
		);
	}
	return {
		nodeId: nodeBody.data.nodeInfo.id,
		projectId: fixture.id
	};
}

async function readFirstPaint(page: import('@playwright/test').Page): Promise<number> {
	return page.evaluate(() => {
		const entry = performance
			.getEntriesByType('paint')
			.find((item) => item.name === 'first-paint') as PerformanceEntry | undefined;
		return entry ? Math.round(entry.startTime) : -1;
	});
}

async function measureScrollSmoothness(page: import('@playwright/test').Page): Promise<{
	averageFrameMs: number;
	framesPerSecond: number;
	samples: number;
}> {
	return page.evaluate(async () => {
		const scrollRoot = document.scrollingElement ?? document.documentElement;
		const intervals: number[] = [];
		let previous = performance.now();
		const start = previous;
		const durationMs = 1200;

		while (performance.now() - start < durationMs) {
			window.scrollBy(0, 900);
			await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
			const now = performance.now();
			intervals.push(now - previous);
			previous = now;
		}

		scrollRoot.scrollTop = 0;
		window.scrollTo(0, 0);
		const averageFrameMs = intervals.reduce((sum, value) => sum + value, 0) / intervals.length;
		return {
			averageFrameMs: Number(averageFrameMs.toFixed(1)),
			framesPerSecond: Number((1000 / averageFrameMs).toFixed(1)),
			samples: intervals.length
		};
	});
}

test('2k fixture loads without crash', async ({ page, request }) => {
	test.setTimeout(150_000);
	const ids = await getFixtureIds(request);
	const base = `/nodes/${ids.nodeId}/projects/${ids.projectId}/artifacts?mediaType=text%2Fmarkdown`;

	await page.goto(base, { waitUntil: 'domcontentloaded', timeout: 60_000 });
	await expect(page.getByRole('heading', { name: 'Artifacts' })).toBeVisible({ timeout: 60_000 });
	await expect(page.getByText(/2000 total/)).toBeVisible();

	const coldFirstPaint = await readFirstPaint(page);

	await page.reload({ waitUntil: 'domcontentloaded', timeout: 60_000 });
	await expect(page.getByRole('heading', { name: 'Artifacts' })).toBeVisible({ timeout: 60_000 });
	await expect(page.getByText(/2000 total/)).toBeVisible();
	const warmFirstPaint = await readFirstPaint(page);

	const scroll = await measureScrollSmoothness(page);

	const search = page.getByPlaceholder('Search title or path…');
	const searchStart = performance.now();
	await search.fill('Scale Artifact 1999');
	await expect(page.getByText(/1 total/)).toBeVisible();
	const searchLatencyMs = Math.round(performance.now() - searchStart);

	expect(coldFirstPaint).toBeGreaterThan(0);
	expect(warmFirstPaint).toBeGreaterThan(0);
	expect(scroll.samples).toBeGreaterThan(0);
	expect(searchLatencyMs).toBeGreaterThan(0);
});
