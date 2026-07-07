package bead

import "testing"

func TestBackendCompositeMatchesTD027(t *testing.T) {
	var _ Backend = (*Store)(nil)
}

func TestReadOnlyBackendComposite(t *testing.T) {
	var _ ReadOnlyBackend = (*Store)(nil)
}
