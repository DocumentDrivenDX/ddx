package server

import (
	"reflect"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/require"
)

// TestServerBeadStoreAccessorsUseBackendInterfaces guards the TD-027
// migration on the server package: the shared store helpers must expose an
// interface, not the concrete *bead.Store.
func TestServerBeadStoreAccessorsUseBackendInterfaces(t *testing.T) {
	backendType := reflect.TypeOf((*bead.Backend)(nil)).Elem()
	concreteType := reflect.TypeOf((*bead.Store)(nil))

	requestMethod := reflect.TypeOf((&Server{}).beadStoreForRequest)
	require.Equal(t, 1, requestMethod.NumIn())
	require.Equal(t, 1, requestMethod.NumOut())
	require.Equal(t, reflect.Interface, requestMethod.Out(0).Kind())
	require.True(t, requestMethod.Out(0).Implements(backendType))
	require.NotEqual(t, concreteType, requestMethod.Out(0))

	storeMethod := reflect.TypeOf((&Server{}).beadStore)
	require.Equal(t, 0, storeMethod.NumIn())
	require.Equal(t, 1, storeMethod.NumOut())
	require.Equal(t, reflect.Interface, storeMethod.Out(0).Kind())
	require.True(t, storeMethod.Out(0).Implements(backendType))
	require.NotEqual(t, concreteType, storeMethod.Out(0))

	var backend bead.Backend = bead.NewStore(t.TempDir())
	require.NotNil(t, backend)
}
