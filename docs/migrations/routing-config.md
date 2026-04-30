# Routing Config Migration

Removed in `ddx-3bd7396a`.

`profile_ladders`, `model_overrides`, and `default_harness` are retired DDx
routing config keys. DDx hard-rejects them at config load so old local config
cannot silently reintroduce DDx-side model/provider routing.

Delete those keys from project and user config. DDx orchestration should request
agent work with effort/power bounds and record the model/power returned by the
agent. The agent owns concrete model/provider routing. Explicit harness,
provider, or model values are operator-supplied passthrough controls only; they
are not replacement routing policy.
