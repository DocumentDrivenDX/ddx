import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

export const FEDERATION_NODES_QUERY = gql`
	query FederationNodes {
		federationNodes {
			id
			nodeId
			name
			url
			status
			ddxVersion
			schemaVersion
			capabilities
			registeredAt
			lastHeartbeat
			writeCapability
			lastError
		}
	}
`

export interface FederationNode {
	id: string
	nodeId: string
	name: string
	url: string
	status: string
	ddxVersion: string
	schemaVersion: string
	capabilities: string[]
	registeredAt: string
	lastHeartbeat: string | null
	writeCapability: boolean
	lastError: string | null
}

interface QueryResult {
	federationNodes: FederationNode[]
}

export const load: PageLoad = async ({ fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	try {
		const data = await client.request<QueryResult>(FEDERATION_NODES_QUERY)
		return { nodes: data.federationNodes, error: null as string | null }
	} catch (e) {
		return { nodes: [] as FederationNode[], error: (e as Error).message }
	}
}
