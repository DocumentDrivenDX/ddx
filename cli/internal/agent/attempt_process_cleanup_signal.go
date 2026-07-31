package agent

// attemptProcessCleanupSignal is the cross-platform signal surfaced after the
// post-attempt descendant cleanup pass. It only carries the fact that live
// descendants survived cleanup; the detailed artifact remains on disk.
type attemptProcessCleanupSignal struct {
	LiveDescendants int
}
