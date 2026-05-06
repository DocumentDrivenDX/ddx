DELETE Store.Init: the exec store no longer defines an Init method or any production call path for it; initialization concerns live elsewhere in the current tree.
DELETE Store.SaveRunRecord: the run-record write path is now implemented by the unexported saveRunRecord helper in cli/internal/exec/bead_runtime.go, and deadcode reports no remaining internal/exec symbol by that name.
DELETE Store.writeRunBundle: run persistence now writes attachments and manifests inline in saveRunRecord; there is no remaining writeRunBundle symbol or call path in cli/internal/exec.
DELETE withPathLock: the path-lock helper is not present in the current cli/internal/exec package, and deadcode reports no internal/exec references to it.
DELETE atomicWriteFile: the atomic file-write helper is not present in the current cli/internal/exec package, and deadcode reports no internal/exec references to it.
