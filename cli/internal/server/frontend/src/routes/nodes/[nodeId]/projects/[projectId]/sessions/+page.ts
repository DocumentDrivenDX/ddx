import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const SESSIONS_QUERY = gql`
	query AgentSessions($first: Int) {
		agentSessions(first: $first) {
			edges {
				node {
					id
					projectId
					beadId
					harness
					model
					effort
					status
					startedAt
					endedAt
					durationMs
					cost
					tokens {
						prompt
						completion
						total
						cached
					}
					outcome
					detail
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

export const SESSION_DETAIL_QUERY = gql`
	query AgentSessionDetail($id: ID!) {
		agentSession(id: $id) {
			id
			prompt
			response
			stderr
		}
	}
`

interface TokenUsage {
	prompt: number | null
	completion: number | null
	total: number | null
	cached: number | null
}

export interface SessionNode {
	id: string
	projectId: string
	beadId: string | null
	harness: string
	model: string
	effort: string
	status: string
	startedAt: string
	endedAt: string | null
	durationMs: number
	cost: number | null
	tokens: TokenUsage | null
	outcome: string | null
	detail: string | null
	prompt?: string | null
	response?: string | null
	stderr?: string | null
}

interface SessionEdge {
	node: SessionNode
	cursor: string
}

interface SessionConnection {
	edges: SessionEdge[]
	pageInfo: { hasNextPage: boolean; endCursor: string | null }
	totalCount: number
}

interface SessionsResult {
	agentSessions: SessionConnection
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<SessionsResult>(SESSIONS_QUERY, { first: 100 })
	const filteredEdges = data.agentSessions.edges.filter(
		(e) => e.node.projectId === params.projectId
	)
	return {
		projectId: params.projectId,
		sessions: {
			...data.agentSessions,
			edges: filteredEdges,
			totalCount: filteredEdges.length
		}
	}
}
