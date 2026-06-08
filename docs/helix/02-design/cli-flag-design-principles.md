# CLI Flag Design Principles

Status: active · Enforced by `cli/cmd/flag_design_test.go`

DDx is operated mostly by humans under stress and by autonomous workers. Both
need flags that are **predictable and discoverable**. This document captures the
rules that prevent "stupid CLI interfaces" — capabilities that are on by default
but cannot be turned off without arcane knowledge.

## The anti-pattern (what we are preventing)

A capability is **on by default** (good — sensible defaults), but the only way to
disable it is one of:

- the unintuitive `--flag=false` form on a *positive*-named flag (e.g. the old
  `ddx work --self-refresh`, which was secretly default-on in watch mode), or
- an undiscoverable env var / config key with **no CLI flag at all** (e.g. the
  automatic update-check network call, previously only `DDX_DISABLE_UPDATE_CHECK`).

Both make a user (or worker) believe the behavior is forced. It is "STUPID".

## The rules

1. **No default-on capability without a discoverable opt-out.** A boolean flag
   may default to `true` *only if* a `--no-<name>` (or `--disable-<name>`) flag
   exists on the same command. Prefer: declare the flag default `false`, make the
   capability on-by-default in code, and expose `--no-<capability>` to disable.

2. **Runtime-default-on still needs a `--no-X`.** If a capability is on by
   default via runtime logic (mode-dependent, like watch mode) rather than the
   flag's declared default, it must STILL expose a `--no-<capability>` opt-out,
   and the positive flag's help must point to it.

3. **Help text must not lie about defaults.** Do not declare `Bool(name, false,
   "...defaults on...")`. If it is on by default, either default it to a
   `--no-X` design (rule 1) or document the exact opt-out in the help.

4. **Every default-on side effect (network calls, self-mutation, auto-commit,
   auto-install, telemetry) needs a CLI off-switch**, not only an env var or
   config key. Env/config opt-outs are fine *in addition*, never *instead*.

The established good example to copy is `--no-review` on `ddx work` / `ddx try`:
review is on by default, with a discoverable `--no-review` opt-out.

## Enforcement

`cli/cmd/flag_design_test.go` walks the entire Cobra command tree on every test
run and fails CI when:

- **RULE 1** — a bool flag defaults to `true` with no `--no-<name>` opt-out on the
  same command.
- **RULE 2** — a positive bool flag whose help advertises default-on behavior
  ("on by default", "defaults on", …) has no `--no-<name>` opt-out.

The check is self-verifying (`TestCLIFlagDesign_CheckCatchesViolations`). Genuine
exceptions go in `allowedExceptions` with a written justification — but the strong
default is to just add the `--no-X` flag.

Rule 4 (off-switch for non-flag side effects) is not mechanically detectable from
flag metadata; reviewers enforce it. When you add a default-on side effect, add
the `--no-X` flag in the same change so the linter covers it thereafter.
