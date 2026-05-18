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

This image intentionally does not bake in provider credentials. The backend
passes known provider environment variables through at runtime.
