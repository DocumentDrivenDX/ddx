# build-fixture-repo

Reusable clean ddx-initialized git repo for tests, integration runs, and
acceptance demos. Avoids polluting the main DDx project when an example
workspace is needed.

## Usage

```bash
scripts/build-fixture-repo.sh <dest> [--profile minimal|standard|multi-project|federated]
```

Cleanup is the caller's responsibility (`rm -rf <dest>`).

### Env

| Var       | Purpose                                                  | Default            |
| --------- | -------------------------------------------------------- | ------------------ |
| `DDX_BIN` | Path to the `ddx` binary used for seeding sample beads. | `ddx` (from PATH) |

## Profiles

| Profile         | Layout                                          | Seeded data                                  |
| --------------- | ----------------------------------------------- | -------------------------------------------- |
| `minimal`       | `<dest>/.ddx/` (config + empty `beads.jsonl`)   | none                                         |
| `standard`      | `<dest>/.ddx/`                                  | 5 mixed-priority sample beads (P0–P3)        |
| `multi-project` | `<dest>/proj-a/`, `<dest>/proj-b/`              | `proj-a` seeded with the standard 5 beads    |
| `federated`     | `<dest>/hub/`, `<dest>/spoke/`                  | `hub` seeded with the standard 5 beads. The federation handshake is **not** registered — wire it up in your test if needed. |

Each project is its own git repo (`git init`, initial commit on `main`).

## Examples

```bash
# Operator walkthrough.
scripts/build-fixture-repo.sh /tmp/ddx-demo --profile standard
cd /tmp/ddx-demo && ddx bead ready

# Cross-project smoke.
scripts/build-fixture-repo.sh /tmp/ddx-mp --profile multi-project
ls /tmp/ddx-mp   # proj-a proj-b
```

## Go test helper

Tests should use `testutils.NewFixtureRepo` instead of calling the script
directly — it auto-cleans via `t.Cleanup` and lazily builds a `ddx` binary
from `cli/` if neither `$DDX_BIN` nor `ddx` on `PATH` is available:

```go
import "github.com/DocumentDrivenDX/ddx/internal/testutils"

func TestX(t *testing.T) {
    projA := testutils.NewFixtureRepo(t, "minimal")
    // ... use projA as a real ddx-initialized project root.
}
```

For `multi-project` / `federated`, the returned path is the parent dir;
sub-projects live underneath (`proj-a`, `proj-b` / `hub`, `spoke`).
