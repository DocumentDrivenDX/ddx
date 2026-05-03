export interface RunDetail {
	id: string
	layer: string
	status: string
	projectID: string | null
	beadId: string | null
	artifactId: string | null
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

	bundleFiles?: BundleFile[]
}

export interface BundleFile {
	path: string
	size: number
	mimeType: string
}

export interface BundleFileContent {
	path: string
	content: string | null
	sizeBytes: number
	truncated: boolean
	mimeType: string
}

export interface ToolCall {
	id: string
	name: string
	seq: number
	ts: string | null
	inputs: string | null
	output: string | null
	truncated: boolean | null
}

export interface SessionDetail {
	id: string
	harness: string
	model: string
	cost: number | null
	billingMode: string
	tokens: { prompt: number | null; completion: number | null; total: number | null; cached: number | null } | null
	status: string
	outcome: string | null
	prompt?: string | null
	response?: string | null
	stderr?: string | null
}
