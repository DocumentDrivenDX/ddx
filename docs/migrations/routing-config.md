# Routing Config Migration

Removed in `ddx-3bd7396a`.

`profile_ladders`, `model_overrides`, `default_harness`, and the old
`agent.routing.*` defaults are retired DDx routing config keys. DDx hard-rejects
them at config load so old local config cannot silently reintroduce DDx-side
model/provider routing.

Delete those keys from project and user config. DDx execution config should
describe queue cadence, retry policy, power bounds, and evidence handling.
Fizeau routing config owns provider/model catalogs, fuzzy matching, fallback
policy, and route health. Explicit harness, provider, or model values are
operator-supplied passthrough controls only; they are not replacement routing
policy and are not filled in from `agent.*` defaults.
