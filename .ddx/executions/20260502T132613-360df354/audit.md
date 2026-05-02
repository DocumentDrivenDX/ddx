# Workers project-scope audit (ddx-4caba860)

## Question
Does the Workers tab show only workers belonging to the currently selected project?

## Findings: scoping is already correctly enforced

### Frontend (`cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.ts`)
The route loader passes the current `projectId` route param into the
`WorkersByProject` query as the `$projectID` variable. There is no
"all projects" surface in this route — every fetch is parameterised by the
selected project.

```graphql
query WorkersByProject($projectID: String!) {
    workersByProject(projectID: $projectID, first: 50) { ... }
    queueAndWorkersSummary(projectId: $projectID) { maxCount }
}
```

The Svelte layout then renders rows directly from `data.workers.edges`
(`+layout.svelte:374`), so what the user sees is exactly what the resolver
returned for that project ID. There is no client-side merging from any
broader source.

### GraphQL resolver (`cli/internal/server/graphql/resolver_agent.go:15`)
```go
func (r *queryResolver) WorkersByProject(ctx context.Context, projectID string, ...) (*WorkerConnection, error) {
    workers := r.State.GetWorkersGraphQL(projectID)
    ...
}
```

### Server state filter (`cli/internal/server/state_graphql.go:268`)
`GetWorkersGraphQL(projectID)` resolves the project's path, then walks
`.ddx/workers/<id>/status.json` and skips any worker whose
`rec.ProjectRoot` differs from the resolved project's path:

```go
if expectedPath != "" && rec.ProjectRoot != expectedPath {
    continue
}
```

### Conclusion
No code change is required for AC #1: the current implementation already
filters workers by `ProjectRoot` against the selected project's path on
the server, and the frontend has no code path that aggregates across
projects.

## Test added (AC #2)
`cli/internal/server/frontend/e2e/workers.spec.ts` — new test
`workers list is scoped to the currently-selected project`. It:

1. Sets up a multi-project fixture with two projects (`project-A`,
   `project-B`), each with disjoint worker sets.
2. Mocks `WorkersByProject` to return per-project workers based on the
   `projectID` GraphQL variable.
3. Navigates to project A's workers route and asserts only project A's
   workers are visible (and project B's are not).
4. Navigates to project B's workers route and asserts the inverse.
5. Asserts the loader requested both project IDs (proving the route
   parameter is threaded into the query variables).
