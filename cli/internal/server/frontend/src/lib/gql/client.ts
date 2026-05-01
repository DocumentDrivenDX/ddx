import { GraphQLClient } from 'graphql-request';

const EMPTY_PAGE_INFO = { hasNextPage: false, endCursor: null };
const FALLBACK_NODE = { id: 'local-node', name: 'Local Node' };
const FALLBACK_PROJECT = { id: 'local-project', name: 'Local Project', path: '/' };
const FALLBACK_PERSONAS = [
	{
		id: 'persona-code-reviewer',
		name: 'code-reviewer',
		roles: ['code-reviewer'],
		description: 'Strict reviewer focused on correctness and safety.',
		body: '# Code Reviewer\n\nYou are a strict reviewer focused on correctness, regressions, and missing tests.',
		source: 'ddx-library',
		bindings: [{ projectId: 'local-project', role: 'code-reviewer', persona: 'code-reviewer' }],
		tags: [],
		filePath: null,
		modTime: null
	},
	{
		id: 'persona-test-engineer',
		name: 'test-engineer',
		roles: ['test-engineer', 'implementer'],
		description: 'Writes focused tests before implementation changes.',
		body: '# Test Engineer\n\nYou turn acceptance criteria into concrete failing tests first.',
		source: 'ddx-library',
		bindings: [],
		tags: [],
		filePath: null,
		modTime: null
	}
];
const FALLBACK_PLUGIN = {
	name: 'helix',
	version: '1.4.2',
	installedVersion: '1.4.2',
	type: 'workflow',
	description: 'HELIX methodology: phases, gates, supervisory dispatch',
	keywords: ['workflow', 'methodology'],
	status: 'installed',
	registrySource: 'builtin',
	diskBytes: 4200000,
	manifest: '{"name":"helix","version":"1.4.2"}',
	skills: ['helix-align', 'helix-plan'],
	prompts: ['drain-queue', 'run-checks'],
	templates: ['FEAT-spec']
};

function fallbackDataForQuery(query: string): object | null {
	const data: Record<string, unknown> = {};

	if (query.includes('nodeInfo')) {
		data.nodeInfo = FALLBACK_NODE;
	}

	if (query.includes('projects')) {
		data.projects = { edges: [{ node: FALLBACK_PROJECT }] };
	}

	if (query.includes('beadsByProject')) {
		data.beadsByProject = {
			edges: [],
			pageInfo: EMPTY_PAGE_INFO,
			totalCount: 0
		};
	}

	if (query.includes('documents')) {
		data.documents = {
			edges: [],
			pageInfo: EMPTY_PAGE_INFO,
			totalCount: 0
		};
	}

	if (query.includes('artifacts')) {
		data.artifacts = {
			edges: [],
			pageInfo: EMPTY_PAGE_INFO,
			totalCount: 0
		};
	}

	if (query.includes('docGraph')) {
		data.docGraph = {
			rootDir: '',
			documents: [],
			warnings: []
		};
	}

	if (query.includes('personas')) {
		data.personas = FALLBACK_PERSONAS;
	}

	if (query.includes('agentSessions')) {
		data.agentSessions = {
			edges: [],
			pageInfo: EMPTY_PAGE_INFO,
			totalCount: 0
		};
	}

	if (query.includes('queueSummary')) {
		data.queueSummary = { ready: 0, blocked: 0, inProgress: 0 };
	}

	if (query.includes('workersByProject')) {
		data.workersByProject = {
			edges: [],
			pageInfo: EMPTY_PAGE_INFO,
			totalCount: 0
		};
	}

	if (query.includes('workerLog')) {
		data.workerLog = { stdout: '', stderr: '' };
	}

	if (query.includes('workerDispatch')) {
		data.workerDispatch = { id: 'worker-preview', state: 'queued', kind: 'execute-loop' };
	}

	if (query.includes('efficacyRows')) {
		data.efficacyRows = [
			{
				rowKey: 'codex|openai|gpt-5',
				harness: 'codex',
				provider: 'openai',
				model: 'gpt-5',
				attempts: 42,
				successes: 40,
				successRate: 0.9524,
				medianInputTokens: 3200,
				medianOutputTokens: 1100,
				medianDurationMs: 28000,
				medianCostUsd: 0.032,
				warning: null
			},
			{
				rowKey: 'claude|anthropic|claude-sonnet-4-6',
				harness: 'claude',
				provider: 'anthropic',
				model: 'claude-sonnet-4-6',
				attempts: 60,
				successes: 57,
				successRate: 0.95,
				medianInputTokens: 4100,
				medianOutputTokens: 1500,
				medianDurationMs: 45000,
				medianCostUsd: 0.047,
				warning: null
			},
			{
				rowKey: 'codex|vidar-omlx|qwen3.6-35b',
				harness: 'codex',
				provider: 'vidar-omlx',
				model: 'qwen3.6-35b',
				attempts: 80,
				successes: 48,
				successRate: 0.6,
				medianInputTokens: 2800,
				medianOutputTokens: 900,
				medianDurationMs: 62000,
				medianCostUsd: null,
				warning: { kind: 'below-adaptive-floor', threshold: 0.7 }
			}
		];
	}

	if (query.includes('efficacyAttempts')) {
		data.efficacyAttempts = { rowKey: '', attempts: [] };
	}

	if (query.includes('comparisons')) {
		data.comparisons = [];
	}

	if (query.includes('comparisonDispatch')) {
		data.comparisonDispatch = { id: 'cmp-preview', state: 'queued', armCount: 0 };
	}

	if (query.includes('projectBindings')) {
		data.projectBindings = '{}';
	}

	if (query.includes('personaBind')) {
		data.personaBind = { ok: true, role: '', persona: '' };
	}

	if (query.includes('pluginsList')) {
		data.pluginsList = [FALLBACK_PLUGIN];
	}

	if (query.includes('pluginDetail')) {
		data.pluginDetail = FALLBACK_PLUGIN;
	}

	if (query.includes('pluginDispatch')) {
		data.pluginDispatch = { id: 'worker-plugin-preview', state: 'queued', action: 'install' };
	}

	if (query.includes('paletteSearch')) {
		data.paletteSearch = { documents: [], beads: [], actions: [], navigation: [] };
	}

	if (query.includes('providerStatuses')) {
		data.providerStatuses = [];
	}

	if (query.includes('harnessStatuses')) {
		data.harnessStatuses = [];
	}

	if (query.includes('defaultRouteStatus')) {
		data.defaultRouteStatus = null;
	}

	if (query.includes('providerTrend')) {
		data.providerTrend = null;
	}

	if (query.includes('NodeRuns') || query.includes('ProjectRuns')) {
		data.runs = {
			edges: [],
			pageInfo: EMPTY_PAGE_INFO,
			totalCount: 0
		};
	}

	if (query.includes('RunDetail')) {
		data.run = null;
	}

	if (query.includes('beadClose')) {
		data.beadClose = {
			id: 'preview-bead',
			title: 'Preview bead',
			status: 'closed',
			priority: 2,
			issueType: 'task'
		};
	}

	return Object.keys(data).length > 0 ? data : null;
}

function isGraphQLEndpoint(input: Parameters<typeof globalThis.fetch>[0]): boolean {
	if (typeof input === 'string') {
		return new URL(input, globalThis.location?.href ?? 'http://localhost').pathname === '/graphql';
	}
	if (input instanceof URL) {
		return input.pathname === '/graphql';
	}
	return new URL(input.url).pathname === '/graphql';
}

async function requestBodyText(
	input: Parameters<typeof globalThis.fetch>[0],
	init?: Parameters<typeof globalThis.fetch>[1]
): Promise<string | null> {
	if (typeof init?.body === 'string') {
		return init.body;
	}
	if (input instanceof Request) {
		return input.clone().text();
	}
	return null;
}

function fallbackGraphQLResponse(data: object): Response {
	return new Response(JSON.stringify({ data }), {
		status: 200,
		headers: { 'content-type': 'application/json' }
	});
}

function delegateInputFor(
	input: Parameters<typeof globalThis.fetch>[0]
): Parameters<typeof globalThis.fetch>[0] {
	if (typeof window === 'undefined' || !isGraphQLEndpoint(input)) {
		return input;
	}
	if (typeof input === 'string' || input instanceof URL) {
		return '/graphql';
	}
	return input;
}

function withStaticPreviewFallback(fetchFn?: typeof globalThis.fetch): typeof globalThis.fetch {
	const delegate = fetchFn ?? globalThis.fetch;
	return async (input, init) => {
		const delegateInput = delegateInputFor(input);
		const response = await delegate(delegateInput, init);
		if (response.status !== 404 || !isGraphQLEndpoint(input)) {
			return response;
		}

		const bodyText = await requestBodyText(input, init);
		if (!bodyText) {
			return response;
		}

		let body: { query?: string };
		try {
			body = JSON.parse(bodyText) as { query?: string };
		} catch {
			return response;
		}

		const data = body.query ? fallbackDataForQuery(body.query) : null;
		return data ? fallbackGraphQLResponse(data) : response;
	};
}

/**
 * Creates a GraphQL HTTP client for queries and mutations.
 *
 * Pass the SvelteKit-provided `fetch` in load functions so requests
 * respect SvelteKit's SSR/CSR fetch instrumentation.
 */
export function createClient(fetchFn?: typeof globalThis.fetch): GraphQLClient {
	const url =
		typeof window !== 'undefined' ? new URL('/graphql', window.location.href).href : '/graphql';
	return new GraphQLClient(url, { fetch: withStaticPreviewFallback(fetchFn) });
}
