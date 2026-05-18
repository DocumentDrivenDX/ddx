# Docker Attempt Build Environments

`docker-clone` gives each `ddx try` attempt a contained runtime. The baseline
image supplies common agent and build tools; a project image supplies expensive,
project-specific setup that should be reused across attempts.

## Image Layers

There are two image layers:

- **Baseline image**: built by the DDx repository with
  `make docker-attempt-runner`, normally tagged `ddx-attempt-runner:dev`. It
  contains Go, git, build tools, Python, Node/npm, Bun, `make`, `jq`, `rg`, and
  the harness shims DDx can mount into attempts.
- **Project image**: built from a repository-owned Dockerfile such as
  `.ddx/attempt-runner.Dockerfile`. It starts with
  `ARG DDX_BASE_IMAGE=ddx-attempt-runner:dev` and `FROM ${DDX_BASE_IMAGE}`, then
  installs SDKs, CLIs, and dependency caches for that repository.

`docker-clone` auto-detects `.ddx/attempt-runner.Dockerfile`. You can override
that with:

```bash
ddx config set executions.docker.project_dockerfile .ddx/attempt-runner.Dockerfile
ddx config set executions.docker.project_context .
ddx config set executions.docker.project_image my-prebuilt-project-attempt:latest
```

If `executions.docker.project_image` is set, DDx uses that image directly and
does not build from the project Dockerfile.

## Runtime Mounts

At runtime DDx bind-mounts the attempt clone at `/work`. Anything baked into an
image under `/work` is hidden by that mount. Put reusable setup under `/opt`,
`/usr/local`, or language-specific cache locations instead.

Writable runtime paths are intentionally separate from host-global temp space:

- `/work` is the per-attempt clone.
- `/work/.gocache` is the per-attempt Go build cache mount.
- `/work/.tmp` is a per-attempt repo-local scratch mount.
- `/ddx-runtime` is DDx-owned runtime state for the attempt.
- `/tmp` is a bounded container tmpfs.

Provider credentials should not be baked into either image. DDx passes known
provider environment variables through and mounts a minimal per-attempt auth
home.

## Dockerfile Pattern

Keep the project Dockerfile cacheable by copying only manifests and lockfiles
before expensive setup:

```dockerfile
ARG DDX_BASE_IMAGE=ddx-attempt-runner:dev
FROM ${DDX_BASE_IMAGE}

WORKDIR /opt/my-project

COPY go.mod go.sum ./
RUN go mod download

COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts --cache /opt/npm-cache && rm -rf node_modules
```

Source files should enter through the per-attempt clone, not through the cached
image layer. For large repositories, add a Dockerfile-specific ignore file next
to the project Dockerfile:

```text
# .ddx/attempt-runner.Dockerfile.dockerignore
**
!go.mod
!go.sum
!package.json
!package-lock.json
```

The ignore file keeps image rebuilds from transferring the whole repository.
In local DDx trials, a correctly scoped project Dockerfile build transferred
only manifest bytes while still reusing the expensive image layers.

## Language Setup Examples

For Go:

```dockerfile
COPY go.mod go.sum ./
RUN go mod download
```

For workspaces or nested modules, copy each module's `go.mod` and `go.sum`
before running `go mod download` in that module.

For Rust:

```dockerfile
ENV RUSTUP_HOME=/opt/rustup CARGO_HOME=/opt/cargo
RUN curl https://sh.rustup.rs -sSf | sh -s -- -y --profile minimal
ENV PATH=/opt/cargo/bin:${PATH}
COPY Cargo.toml Cargo.lock ./
RUN cargo fetch --locked
```

For Java with Maven:

```dockerfile
COPY pom.xml ./
RUN mvn -B dependency:go-offline
```

For Java with Gradle, copy the wrapper, settings, build files, and lockfiles
first, then run the repository's offline dependency task.

For Python:

```dockerfile
COPY requirements.txt ./
RUN python3 -m pip download --dest /opt/pip-wheelhouse -r requirements.txt
ENV PIP_FIND_LINKS=/opt/pip-wheelhouse
```

For `uv` or Poetry, install the tool under `/usr/local` or `/opt`, copy the
lockfile, and prefetch dependencies into an image-owned cache directory.

For Node and Bun:

```dockerfile
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts --cache /opt/npm-cache && rm -rf node_modules
```

Do not rely on `node_modules` baked into the repository path; `/work` hides it.
Use package-manager caches or install global CLIs under `/usr/local`.

## Verification Checklist

Before using a project image to drain a queue:

1. Build the baseline image:

   ```bash
   make docker-attempt-runner
   ```

2. Build the project image from the project root:

   ```bash
   docker build \
     -f .ddx/attempt-runner.Dockerfile \
     -t my-project-attempt:local \
     --build-arg DDX_BASE_IMAGE=ddx-attempt-runner:dev \
     .
   ```

3. Confirm the Docker build context is small. If Docker sends the whole repo,
   tighten `.ddx/attempt-runner.Dockerfile.dockerignore`.
4. Run one preserved attempt first:

   ```bash
   ddx try <bead-id> --attempt-backend docker-clone --no-merge
   ```

5. Check `docker stats` while it runs. Memory, CPU, PIDs, and `/tmp` should be
   bounded by `executions.docker.*`.
6. After interruption or completion, verify the attempt container and runtime
   directories were removed.

## Lessons From Local Trials

Local Docker attempts fixed the main host-contamination failure mode: orphaned
agent and test subprocesses now stay inside the attempt container and are
removed when the container is removed. The Docker run environment also needs a
stable `PATH` and repo-local caches such as `GOCACHE=/work/.gocache`; otherwise
Go-heavy projects can fail inside Docker even though the host environment works.

The current timeout split is still important operationally. `--request-timeout`
limits the agent provider request, but long acceptance gates can keep a
container alive after the provider call returns. Use an outer process timeout or
worker supervision when testing new project images until DDx has a first-class
attempt wall-clock limit.
