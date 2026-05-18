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

WORKDIR /opt/ddx-project

# Install project-specific SDKs, tools, and dependency caches here.
# Keep cached setup outside /work; /work is replaced by the attempt clone.
COPY go.mod go.sum ./
RUN go mod download
```

When that file exists, `docker-clone` builds `ddx-project-attempt-<project>:latest`
with `DDX_BASE_IMAGE` set to `executions.docker.image`, then runs attempts from
that project image. Use `executions.docker.project_dockerfile` or
`executions.docker.project_context` to point at a different repo-owned build
file/context, or `executions.docker.project_image` to use a prebuilt image.
For large repositories, add a Dockerfile-specific ignore file next to the
project Dockerfile (for example `.ddx/attempt-runner.Dockerfile.dockerignore`)
so the cached setup build only transfers dependency manifests and lockfiles.

Use the project Dockerfile for repeatable, cacheable setup work:

- Go: copy `go.mod`/`go.sum` (or workspace manifests) and run `go mod download`.
- Rust: install Rust under `/opt/rustup`/`/opt/cargo`, copy `Cargo.toml` and
  `Cargo.lock`, then run `cargo fetch --locked`.
- Java: copy Maven or Gradle wrapper/lock files, then run the offline dependency
  fetch step such as `mvn dependency:go-offline` or `gradle dependencies`.
- Python: install the project package manager (`uv`, Poetry, or pip tooling)
  under `/usr/local` or `/opt`, then prefetch wheels from `uv.lock`,
  `poetry.lock`, or `requirements.txt`.
- Node/Bun: copy `package.json` plus the lockfile and prefetch package-manager
  caches. Avoid relying on `node_modules` baked under the repo path because the
  runtime `/work` mount hides image contents at that path.

The important Docker pattern is to `COPY` only manifests and lockfiles before
the expensive setup `RUN` step. Source files should enter through the per-attempt
clone, not through the cached image layer.

For a more complete project-image guide and verification checklist, see
[`docs/docker-attempt-build-environments.md`](../../docs/docker-attempt-build-environments.md).

This image intentionally does not bake in provider credentials. The backend
passes known provider environment variables through at runtime and mounts a
minimal per-attempt auth home.
