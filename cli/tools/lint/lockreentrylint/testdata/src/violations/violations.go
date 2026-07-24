package violations

import "bead"

func badNestedWrite(s *bead.Store) error {
	return s.WithLock(func() error {
		return s.WriteAll([]bead.Bead{{ID: "x"}}) // want `Store.WriteAll nested inside WithLock`
	})
}
