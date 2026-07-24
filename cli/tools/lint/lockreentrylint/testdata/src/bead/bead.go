// Package bead is a minimal stub of github.com/DocumentDrivenDX/ddx/internal/bead
// for analysistest fixtures. The analyzer matches type name Store in package
// path suffix /internal/bead or bare "bead".
package bead

type Bead struct {
	ID string
}

type Store struct{}

func (s *Store) WithLock(fn func() error) error { return fn() }

func (s *Store) WriteAll(beads []Bead) error { return nil }

func (s *Store) WriteAllLocked(beads []Bead) error { return nil }
