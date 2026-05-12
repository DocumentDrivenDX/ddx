import { gql } from 'graphql-request'

export const RUN_DETAIL_QUERY = gql`
	query RunDetailExpand($id: ID!) {
		run(id: $id) {
			id
			layer
			status
			projectID
			beadId
			artifactId
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
			prompt
			response
			stderr
			billingMode
			outcome
			detail
			cachedTokens

			bundleFiles {
				path
				size
				mimeType
			}
		}
	}
`

export const RUN_BUNDLE_FILE_QUERY = gql`
	query RunBundleFileFetch($id: ID!, $path: String!) {
		runBundleFile(id: $id, path: $path) {
			path
			content
			sizeBytes
			truncated
			mimeType
		}
	}
`

export const RUN_SESSION_QUERY = gql`
	query RunSessionExpand($id: ID!) {
		agentSession(id: $id) {
			id
			workerId
			harness
			model
			cost
			billingMode
			baseRev
			resultRev
			stdoutPath
			stderrPath
			tokens {
				prompt
				completion
				total
				cached
			}
			status
			outcome
			prompt
			response
			stderr
		}
	}
`

export const RUN_TOOL_CALLS_QUERY = gql`
	query RunToolCallsExpand($id: ID!, $first: Int, $after: String) {
		runToolCalls(id: $id, first: $first, after: $after) {
			edges {
				node {
					id
					seq
					name: tool
					inputs: input
					output
					error
					durationMs
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
