# Practices: e2e-kind

## Requirements (Frame phase)
- Identify all services the application depends on at runtime
- Define what test data is needed to exercise the full feature set
- Determine test scope: smoke tests, integration tests, chaos tests, or all three
- Decide whether tests run from the host or inside the cluster
- Define a known test user/credential for interactive and automated access

## Design
- `kind-config.yaml` at the project root defines the cluster topology
- NodePort mappings expose services to the host — one mapping per service
- Kubernetes manifests for test infrastructure live in `k8s/` or `deploy/`
- Seed script is deterministic, idempotent, and completes in under 30 seconds
- Lifecycle script wraps cluster creation, deployment, seeding, testing, teardown
- Tests connect to services via localhost + NodePort (from host) or service DNS
  (from inside cluster)

## Implementation
- Create `kind-config.yaml` with NodePort mappings for every exposed service
- Write Kubernetes manifests for each dependency (database, cache, broker, auth)
- Use readiness probes and explicit waits between deployment steps
- Write a seed script that populates realistic test data:
  - Fixed UUIDs/IDs for deterministic state
  - `ON CONFLICT DO NOTHING` or equivalent for idempotency
  - Covers all data states the tests need (populated, edge cases)
  - Includes a documented test user
- Write a lifecycle script (`scripts/e2e.sh` or `scripts/demo.ts`) that
  orchestrates the full flow
- Support `KEEP_CLUSTER=1` env var to preserve the cluster for debugging
- Docker image building uses project Dockerfile — same image as production

## Testing
- **Smoke tests**: verify each service is reachable and responding
- **Integration tests**: exercise API endpoints against the real database and
  dependencies
- **E2E tests**: full user workflows from entry point to verification
- **Chaos tests** (optional): inject failures (pod kills, network partitions)
  and verify recovery
- All tests must be runnable locally: `bash scripts/e2e.sh`
- Tests should complete in under 10 minutes (excluding chaos tests)

## Quality Gates
- `kind create cluster` succeeds with the project's `kind-config.yaml`
- All services reach ready state within 60 seconds
- Seed script completes without errors
- Application passes readiness probe after deployment
- Test suite exits 0
- Cluster is cleaned up after tests (unless `KEEP_CLUSTER=1`)

## CI Integration
- Use `helm/kind-action@v1` to create the cluster in GitHub Actions
- Build Docker image and `kind load docker-image` into the cluster
- Run the lifecycle script (same one used locally)
- Upload pod logs and test artifacts on failure
- Always delete the cluster in a `post` or `if: always()` step
- For long-running chaos tests, trigger on schedule (weekly) rather than
  every push

## Maintenance
- When adding a new service dependency, add it to kind-config.yaml NodePorts
  and the Kubernetes manifests
- When the data model changes, update the seed script
- When deployment topology changes, update the kind-config.yaml node layout
- Periodically verify that the kind cluster topology still matches production
  (same services, same networking model)
