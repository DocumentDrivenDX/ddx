import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const RUN_DETAIL_QUERY = gql`
	query RunDetail($id: ID!) {
		run(id: $id) {
			id
			layer
			status
			projectID
			beadId
			parentRunId
			childRunIds
			startedAt
			completedAt
			durationMs

			queueInputs
			stopCondition
			selectedBeadIds

			baseRevision
			resultRevision
			worktreePath
			mergeOutcome
			checkResults

			harness
			provider
			model
			promptSummary
			powerMin
			powerMax
			tokensIn
			tokensOut
			costUsd
			outputExcerpt
			evidenceLinks
		}
	}
`

export interface RunDetail {
	id: string
	layer: string
	status: string
	projectID: string | null
	beadId: string | null
	parentRunId: string | null
	childRunIds: string[]
	startedAt: string | null
	completedAt: string | null
	durationMs: number | null

	queueInputs: string | null
	stopCondition: string | null
	selectedBeadIds: string[] | null

	baseRevision: string | null
	resultRevision: string | null
	worktreePath: string | null
	mergeOutcome: string | null
	checkResults: string | null

	harness: string | null
	provider: string | null
	model: string | null
	promptSummary: string | null
	powerMin: number | null
	powerMax: number | null
	tokensIn: number | null
	tokensOut: number | null
	costUsd: number | null
	outputExcerpt: string | null
	evidenceLinks: string[] | null
}

const PARENT_RUN_QUERY = gql`
	query ParentRunParent($id: ID!) {
		run(id: $id) {
			parentRunId
		}
	}
`

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<{ run: RunDetail | null }>(RUN_DETAIL_QUERY, {
		id: params.runId
	})

	let grandparentRunId: string | null = null
	if (data.run?.layer === 'run' && data.run.parentRunId) {
		const parentData = await client.request<{ run: { parentRunId: string | null } | null }>(
			PARENT_RUN_QUERY,
			{ id: data.run.parentRunId }
		)
		grandparentRunId = parentData.run?.parentRunId ?? null
	}

	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		runId: params.runId,
		run: data.run,
		grandparentRunId
	}
}
