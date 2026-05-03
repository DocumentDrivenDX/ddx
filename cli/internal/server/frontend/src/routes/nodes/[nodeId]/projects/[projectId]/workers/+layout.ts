import type { LayoutLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const WORKERS_QUERY = gql`
	query WorkersByProject($projectID: String!) {
		workersByProject(projectID: $projectID, first: 50) {
			edges {
				node {
					id
					kind
					state
					status
					harness
					model
					currentBead
					attempts
					successes
					failures
					startedAt
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
			totalCount
		}
		queueAndWorkersSummary(projectId: $projectID) {
			maxCount
		}
	}
`

// ADR-022 step 5b: surface the worker_ingest derived view alongside the
// in-process worker registry. reportedWorkers is unscoped on the server, so
// the loader filters by the current project's path on the client. The
// `project` field is a project ROOT path (from the trusted-peer identity
// envelope), not the project ID — so we resolve the path from /api/projects.
const REPORTED_WORKERS_QUERY = gql`
	query ReportedWorkersByProject {
		reportedWorkers {
			id
			project
			harness
			state
			lastEventAt
			mirrorFailuresCount
			hadDroppedBackfill
			currentBead
			currentAttempt
		}
	}
`

interface WorkerNode {
	id: string
	kind: string
	state: string
	status: string | null
	harness: string | null
	model: string | null
	currentBead: string | null
	attempts: number | null
	successes: number | null
	failures: number | null
	startedAt: string | null
}

interface WorkerEdge {
	node: WorkerNode
	cursor: string
}

interface WorkerConnection {
	edges: WorkerEdge[]
	pageInfo: { hasNextPage: boolean; endCursor: string | null }
	totalCount: number
}

interface WorkersResult {
	workersByProject: WorkerConnection
	queueAndWorkersSummary: { maxCount: number | null }
}

export interface ReportedWorker {
	id: string
	project: string
	harness: string
	state: string
	lastEventAt: string
	mirrorFailuresCount: number
	hadDroppedBackfill: boolean
	currentBead: string | null
	currentAttempt: string | null
}

interface ReportedWorkersResult {
	reportedWorkers: ReportedWorker[]
}

interface ProjectsApiEntry {
	id: string
	name?: string
	path?: string
}

async function fetchProjectPath(
	fetchImpl: typeof globalThis.fetch,
	projectId: string
): Promise<string | null> {
	try {
		const resp = await fetchImpl('/api/projects')
		if (!resp.ok) return null
		const list = (await resp.json()) as ProjectsApiEntry[]
		const match = list.find((p) => p.id === projectId)
		return match?.path ?? null
	} catch {
		return null
	}
}

export const load: LayoutLoad = async ({ params, fetch }) => {
	const fetchImpl = fetch as unknown as typeof globalThis.fetch
	const client = createClient(fetchImpl)

	const [data, reportedRaw, projectPath] = await Promise.all([
		client.request<WorkersResult>(WORKERS_QUERY, { projectID: params.projectId }),
		client
			.request<ReportedWorkersResult>(REPORTED_WORKERS_QUERY)
			.catch(() => ({ reportedWorkers: [] as ReportedWorker[] })),
		fetchProjectPath(fetchImpl, params.projectId)
	])

	const reportedAll = reportedRaw.reportedWorkers ?? []
	const reported = projectPath
		? reportedAll.filter((w) => w.project === projectPath)
		: reportedAll

	return {
		projectId: params.projectId,
		projectPath,
		workers: data.workersByProject,
		maxCount: data.queueAndWorkersSummary?.maxCount ?? null,
		reportedWorkers: reported
	}
}
