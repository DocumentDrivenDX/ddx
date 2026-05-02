import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'
import { readState } from '$lib/urlState'
import type { GroupBy } from './grouping'

export const ARTIFACTS_QUERY = gql`
	query Artifacts($projectID: ID!, $first: Int, $after: String, $mediaType: String, $search: String, $sort: ArtifactSort, $staleness: String, $phase: String, $prefix: [String!]) {
		artifacts(projectID: $projectID, first: $first, after: $after, mediaType: $mediaType, search: $search, sort: $sort, staleness: $staleness, phase: $phase, prefix: $prefix) {
			edges {
				node {
					id
					path
					title
					mediaType
					staleness
				}
				cursor
				snippet
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
	snippet?: string | null
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

export const SORT_OPTIONS = [
	{ value: 'PATH', label: 'Path' },
	{ value: 'TITLE', label: 'Title' },
	{ value: 'MODIFIED', label: 'Last modified' },
	{ value: 'DEPS_COUNT', label: 'Dependency count' },
	{ value: 'ID', label: 'ID' }
] as const

export type ArtifactSort = (typeof SORT_OPTIONS)[number]['value']
export const DEFAULT_SORT: ArtifactSort = 'PATH'

const SORT_VALUES = new Set(SORT_OPTIONS.map((o) => o.value))

export function parseSort(raw: string | null): ArtifactSort {
	return raw && SORT_VALUES.has(raw as ArtifactSort) ? (raw as ArtifactSort) : DEFAULT_SORT
}

export const STALENESS_OPTIONS = ['fresh', 'stale', 'missing'] as const
export type Staleness = (typeof STALENESS_OPTIONS)[number]
const STALENESS_SET = new Set<string>(STALENESS_OPTIONS)

export function parseStaleness(raw: string | null): Staleness | null {
	return raw && STALENESS_SET.has(raw) ? (raw as Staleness) : null
}

export const load: PageLoad = async ({ params, url, fetch }) => {
	const state = readState(url.searchParams)
	const mediaType = state.mediaType
	const q = state.q
	const groupBy: GroupBy = state.groupBy
	const sort = parseSort(state.sort)
	const staleness = parseStaleness(state.staleness)
	const phase = state.phase
	const prefix = state.prefix

	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<ArtifactsResult>(ARTIFACTS_QUERY, {
		projectID: params.projectId,
		first: PAGE_SIZE,
		mediaType: mediaType ?? undefined,
		search: q ? q : undefined,
		sort,
		staleness: staleness ?? undefined,
		phase: phase ?? undefined,
		prefix: prefix.length > 0 ? prefix : undefined
	})
	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		artifacts: data.artifacts,
		mediaType,
		q,
		groupBy,
		sort,
		staleness,
		phase,
		prefix
	}
}
