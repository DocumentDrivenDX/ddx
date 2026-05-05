DELETE internal/registry/manifest.go:126 MarshalPackage - no production caller exists; function was test-only and removing it eliminates the dead symbol without changing runtime behavior.
