import type { LayoutLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

export const ssr = false;

const NODE_INFO_QUERY = gql`
	query NodeInfo {
		nodeInfo {
			id
			name
		}
	}
`

interface NodeInfoResult {
	nodeInfo: {
		id: string
		name: string
	}
}

export const load: LayoutLoad = async ({ fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<NodeInfoResult>(NODE_INFO_QUERY)
	return { nodeInfo: data.nodeInfo }
}
