package graphql

import (
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func projectStatePath(projectRoot string, elems ...string) string {
	return ddxroot.JoinProject(projectRoot, elems...)
}

func projectBeadStore(projectRoot string) *bead.Store {
	return bead.NewStore(projectStatePath(projectRoot))
}
