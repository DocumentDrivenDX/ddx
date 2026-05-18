# DDx Attempt Runner Image

Baseline container image for:

```bash
ddx try <bead-id> --attempt-backend docker-clone
```

The Docker backend mounts the current `ddx` executable into the container at
`/usr/local/bin/ddx`, mounts the local attempt clone at `/work`, and runs
`ddx run` from there. This image provides the toolchain expected by the DDx
repo's common acceptance gates: Go, git, build tools, Python, Node/npm, Bun,
`make`, `jq`, `ripgrep`, and related shell utilities.

Build it from the repository root:

```bash
make docker-attempt-runner
```

Then point DDx at the image:

```bash
./cli/build/ddx config set executions.docker.image ddx-attempt-runner:dev
./cli/build/ddx config set executions.docker.memory 8g
./cli/build/ddx config set executions.docker.cpus 4
```

For early trials, run with `--no-merge`:

```bash
./cli/build/ddx try <bead-id> --attempt-backend docker-clone --no-merge
```

Projects can add a cached setup layer at `.ddx/attempt-runner.Dockerfile`:

```dockerfile
ARG DDX_BASE_IMAGE=ddx-attempt-runner:dev
FROM ${DDX_BASE_IMAGE}

# Install project-specific SDKs, tools, and dependency caches here.
```

When that file exists, `docker-clone` builds `ddx-project-attempt-<project>:latest`
with `DDX_BASE_IMAGE` set to `executions.docker.image`, then runs attempts from
that project image. Use `executions.docker.project_dockerfile` or
`executions.docker.project_context` to point at a different repo-owned build
file/context, or `executions.docker.project_image` to use a prebuilt image.
For large repositories, add a Dockerfile-specific ignore file next to the
project Dockerfile (for example `.ddx/attempt-runner.Dockerfile.dockerignore`)
so the cached setup build only transfers dependency manifests and lockfiles.

This image intentionally does not bake in provider credentials. The backend
passes known provider environment variables through at runtime and mounts a
minimal per-attempt auth home.
