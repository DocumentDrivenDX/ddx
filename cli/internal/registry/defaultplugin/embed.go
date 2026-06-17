// Package defaultplugin holds the embedded default `ddx` plugin package.
//
// The embedded `library/` tree is a minimal package consumed by the registry
// installer: package.yaml plus skills/ddx. It keeps `ddx init` and worker skill
// discovery offline-safe without baking optional library assets into the
// binary. See docs/helix/02-design/adr/ADR-027-skill-install-topology.md.
package defaultplugin

import (
	"embed"
	"io/fs"
)

//go:embed all:library
var embedded embed.FS

// FS returns a filesystem rooted at the embedded default-plugin package
// (the equivalent of the on-disk `library/` directory). The returned fs.FS
// is suitable for passing to registry.InstallPackageFromFS.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "library")
	if err != nil {
		// embed roots are verified at compile time; a sub failure means the
		// embedded tree shape changed under us.
		panic(err)
	}
	return sub
}
