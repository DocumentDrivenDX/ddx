# Routing Config Migration

Removed in `ddx-3bd7396a`.

`profile_ladders`, `model_overrides`, `default_harness`, and the old
`agent.routing.*` defaults are retired DDx routing config keys. DDx hard-rejects
them at config load so old local config cannot silently reintroduce DDx-side
model/provider routing.

Delete those keys from project and user config. DDx execution config should
describe queue cadence, retry policy, power bounds, and evidence handling.
Fizeau routing config owns provider/model catalogs, fuzzy matching, fallback
policy, route health, and model alias resolution. DDx execution config owns
`run` / `try` / `work` policy, while Fizeau routing config owns the concrete
route. Explicit `--harness`, `--provider`, and `--model` values are
operator-supplied passthrough controls only; they are not replacement routing
policy and are not filled in from `agent.*` defaults.

See ADR-024 for retry and review policy: DDx may change `MinPower`,
`MaxPower`, and request facts, but Fizeau chooses the concrete route. DDx never
normalizes, substitutes, widens, or fuzzy-matches explicit passthrough values.
