// Package defaultplugin holds the embedded default `ddx` plugin package.
//
// The embedded `library/` tree is rooted at the package layout consumed by
// the registry installer. It lets `ddx init` install the default plugin
// offline through the same code path as a remote install. See
// docs/helix/02-design/plan-2026-05-13-ddx-skill-package-layout.md.
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
