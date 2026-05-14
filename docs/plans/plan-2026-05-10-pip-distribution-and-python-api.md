---
ddx:
  id: plan-2026-05-10-pip-distribution-and-python-api
---
# Pip Distribution + Auto-Generated Python API

Date: 2026-05-10
Status: Drafted (multi-model reviewed: opus); requires further user direction before bead breakdown

## Summary

Make `ddx` installable via pip and expose its CLI surface as an auto-generated Python module that's introspectable enough to drive Jupyter tab completion, `?` help, and IDE/type-checker support. Built on the standard "binary in a per-platform wheel + tiny Python shim" pattern (ruff, uv, esbuild ship this way), with a Cobra-introspect step driving build-time codegen of `.py` wrappers.

## Motivation

Two distinct user needs:

1. **`pip install ddx`** so Python users — particularly Databricks notebook users — can install ddx without curl-pipe-bash or homebrew. Lower-friction adoption in the data-science / Python world.
2. **Native Python API** (`import ddx; ddx.bead.create(...)`) that's automatically derived from the CLI surface so it never drifts. Introspection has to be load-bearing for the Jupyter/IDE story — runtime `__getattr__` magic doesn't drive tab completion or Pyright; real symbols do.

The Databricks-deployment angle (separate plan) reframes which subset of the API is meaningful in a notebook context, but the distribution channel and the API generator are useful in their own right.

## Architecture

### Distribution

```
python/
  pyproject.toml           # hatchling backend, per-platform wheel tags
  src/ddx/
    __init__.py            # exposes api + main
    _bin.py                # locates bundled binary
    __main__.py            # CLI entry point — subprocess.run + sys.exit
    api/                   # GENERATED — do not edit
      __init__.py
      bead.py
      agent.py
      ...
      _runtime.py          # _invoke(argv, json=True) helper
  scripts/
    codegen.py             # consumes `ddx __introspect`, writes api/
  codegen/
    output_formats.yaml    # per-command: json | ndjson | text | streaming
    databricks_safe.yaml   # allowlist of commands that work in Databricks
```

- Per-platform wheels via `cibuildwheel` matrix (NOT hand-rolled — handles musllinux, manylinux, macOS arm64/x86_64, Windows arm64).
- Each wheel contains exactly one `_bin/ddx` binary for its platform.
- `[project.scripts] ddx = "ddx.__main__:main"` runs `subprocess.run([binary, *sys.argv[1:]]); sys.exit(rc)` — NOT `os.execv` (broken on Windows).
- Explicit SIGINT/SIGTERM forwarding so Jupyter "interrupt kernel" doesn't orphan `ddx work` subprocesses.
- Sentinel for install-method detection: Go build for the wheel uses `-ldflags "-X main.installMethod=pip"`. `ddx upgrade` reads this and refuses to self-replace, pointing at `pip install -U ddx` instead. NOT path-based detection (pipx/--user/conda/uv all break path heuristics).

### Auto-generated Python API

- Add hidden `ddx __introspect` Cobra command (~80 LOC, one new file). Walks `rootCmd.Commands()` recursively, emits JSON: command name, short/long help, positional args, flags (name, type, default, help). Sole new Go surface — by user constraint, the Python machinery should not significantly grow Go.
- `python/scripts/codegen.py` runs `ddx __introspect`, generates real `.py` files under `src/ddx/api/`. One module per top-level command group, one function per leaf command.
- Codegen runs as hatch build hook BEFORE each wheel is packed. Cross-compiled wheels reuse the host's introspection JSON (command tree is platform-invariant).
- Each generated function: real signature with kwargs typed from flag types, docstring lifted from Cobra's `cmd.Long`, dispatches via `_runtime._invoke(argv, flags=...)`.
- Output-format mapping lives in `python/codegen/output_formats.yaml` (NOT in Go-side annotations — keeps Go untouched). Missing entries default to "text + return CompletedProcess" — works automatically; promote to typed JSON via yaml edit.
- Streaming commands (`ddx work`, `legacy agent work`) marked in the yaml; codegen emits an iterator/Popen-returning variant.
- Flag mangling: `--no-merge` → `no_merge`. Bool `True` → emit, `False` → omit (no `--no-no-merge`).
- Required for notebook UX (added per opus review):
  - Every generated function takes `cwd=` (default `os.getcwd()`), `stdin=`, `env=`.
  - Top-level `ddx.context(cwd=..., env=...)` context manager so notebook users don't thread `cwd=` through every call.
  - `--json` is NOT a heuristic — `output_formats.yaml` declares per-command `json | ndjson | text | streaming`. Avoids silent decode failures and notebook OOM on large outputs.

## Multi-model review (opus)

Five must-fix issues addressed in design above:

1. `os.execv` is wrong on Windows → use `subprocess.run` everywhere.
2. CWD/stdin/env are missing → first-class kwargs + `ddx.context()`.
3. `--json` heuristic silently lies → declarative per-command output format, typed `DdxOutputError` on decode failure.
4. Hand-rolled wheel matrix misses musllinux + win_arm64 → use `cibuildwheel`.
5. `ddx upgrade` path-based collision detection is brittle → use build-time `-ldflags` sentinel.

Plus secondary fixes: Cobra `Flags().VisitAll` includes inherited persistent flags (dedupe required); cross-compiled wheels can't introspect themselves (run on host once); checksum check at Python import to detect manual binary swaps.

## Open questions

- Build Part B (the Python API) at all, or ship Part A (distribution) first and gate Part B on actual user pull? Opus explicitly recommends shipping A alone and validating with target users before investing the codegen budget. The Databricks-deployment plan changes this calculus — see that plan.
- If we build B, is the per-module-codegen route (~50 generated `.py` files) worth the maintenance vs. one `.pyi` stub + runtime `__getattr__` dispatch? Pyright/mypy read `.pyi` exclusively; the only loss is `func??` showing real source. Worth a half-day spike before committing.

## Status

Ready to bead-break if user confirms direction. Hold pending the Databricks-deployment plan (which significantly changes Part B's value calculus — pure-Python HTTP client to ddx-server is a different shape than subprocess-wrapping a local binary).
