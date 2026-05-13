package legacysurface // want `routinglint: DDx-owned cli/internal/agent subpackages are retired`

// This package exists only as an analysistest fixture. The analyzer
// must reject any new DDx-owned subpackage beneath cli/internal/agent.
type Marker struct{}
