# Concern: E2E Testing with Kind Clusters

## Category
testing

## Areas
api, infra

## Components

- **Cluster runtime**: kind (Kubernetes in Docker)
- **Cluster config**: `kind-config.yaml` with NodePort mappings for service access
- **Infrastructure manifests**: Kubernetes YAML or Helm charts for all services
- **Test data seeding**: Deterministic seed scripts that populate realistic data
- **Test harness**: Language-appropriate test framework running against the live cluster
- **Lifecycle management**: Scripts to create, seed, test, and tear down the cluster

## Constraints

- Tests run against a real kind cluster, not mocks — services are real containers
  with real networking
- Every service the application depends on must be deployed in the cluster
  (databases, caches, message brokers, auth providers)
- Test data must be deterministic and idempotent — re-running the seed produces
  the same state
- Tests must be self-contained — they create their cluster, seed data, run, and
  clean up
- NodePort mappings in `kind-config.yaml` provide localhost access to services
  without port-forwarding
- The cluster is ephemeral — created for the test run, destroyed after
- Tests must work in both local development and CI environments

## Kind Configuration Pattern

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: <project>-test
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30000
        hostPort: <app-port>
        protocol: TCP
      - containerPort: 30432
        hostPort: 5432
        protocol: TCP
```

NodePort services in the cluster map to `containerPort` values; `hostPort`
is how tests connect from the host.

## Service Stack Pattern

Deploy services in dependency order:

1. **Namespace** — isolate test resources
2. **Databases** (PostgreSQL, Redis, ClickHouse) — stateful services first
3. **Message brokers** (Redpanda/Kafka) — if the app uses event streaming
4. **Auth** (Keycloak, mock OIDC) — if the app uses authentication
5. **Wait for readiness** — probe each service before proceeding
6. **Migrations** — run schema migrations as a Kubernetes Job
7. **Seed data** — run seed script as a Kubernetes Job or from the host
8. **Application** — deploy the app last, after infrastructure is healthy

Use Kubernetes readiness probes or explicit health checks (`pg_isready`,
HTTP `/health`) between steps.

## Test Data Requirements

The seed must produce a realistic, non-trivial dataset:

- **Not empty** — empty state tells you nothing about whether the app works
- **Not minimal** — include enough data to exercise pagination, search,
  filtering, and edge cases
- **Deterministic** — use fixed UUIDs, fixed seeds for random generators,
  `ON CONFLICT DO NOTHING` for idempotency
- **Documented** — include a known test user/credential for interactive
  testing and a data manifest describing what was seeded
- **Fast** — seed should complete in under 30 seconds

### Seed approaches by project type

| Pattern | Example | When to use |
|---------|---------|-------------|
| **SQL seed script** | `scripts/seed.sql` | Simple schemas, no app logic in seeding |
| **App seed command** | `bun run seed`, `cargo run --bin seed` | Seed requires app business logic |
| **Kubernetes Job** | `k8s/seed-job.yaml` | Seed runs inside the cluster network |
| **Test fixture files** | `tests/fixtures/*.json` | Data loaded by test harness at runtime |

## Test Harness Patterns

### From the host (most common)

Tests run on the host machine, connecting to cluster services via NodePorts:

```bash
# Stand up cluster
kind create cluster --config kind-config.yaml
kubectl apply -f k8s/
# Wait for readiness
kubectl wait --for=condition=ready pod -l app=postgres --timeout=60s
# Seed
kubectl apply -f k8s/seed-job.yaml
kubectl wait --for=condition=complete job/seed --timeout=60s
# Test
DATABASE_URL=postgres://user:pass@localhost:5432/testdb npm test
# Tear down
kind delete cluster --name <project>-test
```

### Inside the cluster

Tests run as Kubernetes Jobs inside the cluster, using service DNS:

```bash
kubectl apply -f k8s/test-job.yaml
kubectl wait --for=condition=complete job/e2e-tests --timeout=300s
kubectl logs job/e2e-tests
```

### Chaos testing (advanced)

Tests inject failures while the application is running:

- Pod kills: `kubectl delete pod <name> --grace-period=0`
- Network partitions: NetworkPolicy rules
- Rolling restarts: `kubectl rollout restart deployment/<name>`
- Recovery verification: confirm the app recovers without data loss

## Lifecycle Script Pattern

Wrap the full lifecycle in a script:

```bash
#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="<project>-test"

# Create or reuse
if ! kind get clusters | grep -q "$CLUSTER_NAME"; then
  kind create cluster --config kind-config.yaml
fi

# Build and load app image
docker build -t <project>:test .
kind load docker-image <project>:test --name "$CLUSTER_NAME"

# Deploy infrastructure
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/redis.yaml
kubectl wait --for=condition=ready pod -l app=postgres -n <ns> --timeout=60s

# Migrate and seed
kubectl apply -f k8s/migrate-job.yaml -n <ns>
kubectl wait --for=condition=complete job/migrate -n <ns> --timeout=120s
kubectl apply -f k8s/seed-job.yaml -n <ns>
kubectl wait --for=condition=complete job/seed -n <ns> --timeout=60s

# Deploy app
kubectl apply -f k8s/app.yaml -n <ns>
kubectl wait --for=condition=ready pod -l app=<project> -n <ns> --timeout=60s

# Run tests
<test-command>

# Cleanup (optional — keep for debugging with KEEP_CLUSTER=1)
if [[ "${KEEP_CLUSTER:-0}" != "1" ]]; then
  kind delete cluster --name "$CLUSTER_NAME"
fi
```

## CI Integration

```yaml
# GitHub Actions example
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: helm/kind-action@v1
        with:
          config: kind-config.yaml
          cluster_name: <project>-test
      - run: docker build -t <project>:test .
      - run: kind load docker-image <project>:test
      - run: bash scripts/e2e-setup.sh
      - run: <test command>
      - if: failure()
        run: kubectl logs -l app=<project> --tail=100
      - run: kind delete cluster --name <project>-test
        if: always()
```

Upload logs and test artifacts on failure for debugging.

## When to use

Any project that deploys to Kubernetes and needs integration or e2e tests
against the real service stack. Particularly valuable when:

- The app has multiple services that interact (database + cache + broker)
- Authentication/authorization requires a real identity provider
- Data migrations must be tested against a real database
- Chaos/resilience testing requires pod lifecycle control
- You want tests to match the production deployment topology

## ADR References
