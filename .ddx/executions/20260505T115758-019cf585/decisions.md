DELETE internal/federation/persistence.go:41 LoadState — the current tree no longer defines this wrapper; production call sites use registry.LoadState or federation.LoadStateFrom instead.
DELETE internal/federation/persistence.go:50 SaveState — the current tree no longer defines this wrapper; production call sites use registry.SaveState or federation.SaveStateTo instead.
