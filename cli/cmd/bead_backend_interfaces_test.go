package cmd

import (
	"reflect"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/require"
)

// TestCmdBeadUsesBackendInterfaces guards the TD-027 command-caller
// migration: the shared factory accessors most of cmd/bead.go calls into
// must return bead.Backend (or a composite that includes it), not the
// concrete *bead.Store. beadStoreConcrete is the documented exception used
// only by call sites that need members outside the Backend composite
// (file/dir paths, ReadyExecution, ExportToFile, ClaimLease, etc.).
func TestCmdBeadUsesBackendInterfaces(t *testing.T) {
	f := &CommandFactory{}
	backendType := reflect.TypeOf((*bead.Backend)(nil)).Elem()

	storeMethod := reflect.TypeOf(f.beadStore)
	require.Equal(t, 0, storeMethod.NumIn())
	require.Equal(t, 1, storeMethod.NumOut())
	require.Equal(t, backendType, storeMethod.Out(0),
		"beadStore must return bead.Backend, not a concrete *bead.Store")

	statusMethod := reflect.TypeOf(f.beadStatusStore)
	require.Equal(t, 1, statusMethod.NumOut())
	statusOut := statusMethod.Out(0)
	require.Equal(t, reflect.Interface, statusOut.Kind(),
		"beadStatusStore must return an interface, not a concrete *bead.Store")
	require.True(t, statusOut.Implements(backendType),
		"beadStatusStore's return type must compose bead.Backend")

	concreteMethod := reflect.TypeOf(f.beadStoreConcrete)
	require.Equal(t, reflect.TypeOf((*bead.Store)(nil)), concreteMethod.Out(0),
		"beadStoreConcrete is the documented concrete-store exception for members outside bead.Backend")

	var backend bead.Backend = bead.NewStore(t.TempDir())
	require.NotNil(t, backend)
}
