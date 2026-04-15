import { GraphQLClient } from 'graphql-request'

/**
 * Creates a GraphQL HTTP client for queries and mutations.
 *
 * Pass the SvelteKit-provided `fetch` in load functions so requests
 * respect SvelteKit's SSR/CSR fetch instrumentation.
 */
export function createClient(fetchFn?: typeof globalThis.fetch): GraphQLClient {
	return new GraphQLClient('/graphql', fetchFn ? { fetch: fetchFn } : undefined)
}
