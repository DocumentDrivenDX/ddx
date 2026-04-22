import { GraphQLClient } from 'graphql-request';

const EMPTY_PAGE_INFO = { hasNextPage: false, endCursor: null };
const FALLBACK_NODE = { id: 'local-node', name: 'Local Node' };
const FALLBACK_PROJECT = { id: 'local-project', name: 'Local Project', path: '/' };

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

	if (query.includes('docGraph')) {
		data.docGraph = {
			rootDir: '',
			documents: [],
			warnings: []
		};
	}

	if (query.includes('personas')) {
		data.personas = {
			edges: [],
			pageInfo: EMPTY_PAGE_INFO,
			totalCount: 0
		};
	}

	if (query.includes('agentSessions')) {
		data.agentSessions = {
			edges: [],
			pageInfo: EMPTY_PAGE_INFO,
			totalCount: 0
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

function withStaticPreviewFallback(fetchFn?: typeof globalThis.fetch): typeof globalThis.fetch {
	const delegate = fetchFn ?? globalThis.fetch;
	return async (input, init) => {
		const response = await delegate(input, init);
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
