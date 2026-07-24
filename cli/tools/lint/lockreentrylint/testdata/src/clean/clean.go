package clean

import "bead"

func okLocked(s *bead.Store) error {
	return s.WithLock(func() error {
		return s.WriteAllLocked([]bead.Bead{{ID: "x"}})
	})
}

func okOuterWriteAll(s *bead.Store) error {
	return s.WriteAll([]bead.Bead{{ID: "y"}})
}
