import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const DOCUMENT_BY_PATH = gql`
	query DocumentByPath($path: String!) {
		documentByPath(path: $path) {
			path
			content
		}
	}
`

interface DocumentByPathResult {
	documentByPath: { path: string; content: string } | null
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	try {
		const data = await client.request<DocumentByPathResult>(DOCUMENT_BY_PATH, {
			path: params.path
		})
		if (!data.documentByPath) {
			return { path: params.path, content: null }
		}
		return {
			path: data.documentByPath.path,
			content: data.documentByPath.content
		}
	} catch {
		return { path: params.path, content: null }
	}
}
