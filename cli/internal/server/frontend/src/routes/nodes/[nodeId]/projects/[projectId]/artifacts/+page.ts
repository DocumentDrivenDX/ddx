import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'
import { readState } from '$lib/urlState'
import type { GroupBy } from './grouping'

export const ARTIFACTS_QUERY = gql`
	query Artifacts($projectID: ID!, $first: Int, $after: String, $mediaType: String) {
		artifacts(projectID: $projectID, first: $first, after: $after, mediaType: $mediaType) {
			edges {
				node {
					id
					path
					title
					mediaType
					staleness
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

export interface ArtifactNode {
	id: string
	path: string
	title: string
	mediaType: string
	staleness: string
}

export interface ArtifactEdge {
	node: ArtifactNode
	cursor: string
}

export interface PageInfo {
	hasNextPage: boolean
	endCursor: string | null
}

export interface ArtifactConnection {
	edges: ArtifactEdge[]
	pageInfo: PageInfo
	totalCount: number
}

interface ArtifactsResult {
	artifacts: ArtifactConnection
}

export const PAGE_SIZE = 50

export const load: PageLoad = async ({ params, url, fetch }) => {
	const state = readState(url.searchParams)
	const mediaType = state.mediaType
	const q = state.q
	const groupBy: GroupBy = state.groupBy

	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<ArtifactsResult>(ARTIFACTS_QUERY, {
		projectID: params.projectId,
		first: PAGE_SIZE,
		mediaType: mediaType ?? undefined
	})
	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		artifacts: data.artifacts,
		mediaType,
		q,
		groupBy
	}
}
