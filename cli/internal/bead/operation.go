package bead

// Operation is a typed mutation applied to a bead snapshot.
type Operation interface {
	Apply(b *Bead) error
}
