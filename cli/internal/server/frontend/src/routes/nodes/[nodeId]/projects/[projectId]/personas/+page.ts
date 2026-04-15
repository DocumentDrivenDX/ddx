import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const PERSONAS_QUERY = gql`
	query Personas($first: Int) {
		personas(first: $first) {
			edges {
				node {
					id
					name
					roles
					description
					tags
					content
					filePath
					modTime
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
			totalCount
		}
	}
`

export interface PersonaNode {
	id: string
	name: string
	roles: string[]
	description: string
	tags: string[]
	content: string
	filePath: string | null
	modTime: string | null
}

interface PersonaEdge {
	node: PersonaNode
	cursor: string
}

interface PersonaConnection {
	edges: PersonaEdge[]
	pageInfo: { hasNextPage: boolean; endCursor: string | null }
	totalCount: number
}

interface PersonasResult {
	personas: PersonaConnection
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<PersonasResult>(PERSONAS_QUERY, { first: 100 })
	return {
		projectId: params.projectId,
		personas: data.personas
	}
}
