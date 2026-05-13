package bead

// Operation is a typed mutation applied to a bead snapshot.
type Operation interface {
	Apply(b *Bead) error
}

// MutateFunc adapts an ad-hoc bead mutation into an Operation.
type MutateFunc func(*Bead) error

// Apply executes the wrapped mutation.
func (m MutateFunc) Apply(b *Bead) error {
	return m(b)
}
