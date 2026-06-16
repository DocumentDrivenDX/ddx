package cmd

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/spf13/cobra"
)

const gracefulCancelMessage = "Cancel received, shutting down gracefully"

// newGracefulCancelAnnouncer returns a one-shot notifier for root-level
// cancellation. It prints the operator-visible shutdown line exactly once and
// then calls stop so a second SIGINT/SIGTERM falls back to the default signal
// disposition and can still kill a wedged cleanup path.
func newGracefulCancelAnnouncer(out io.Writer, stop func()) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			_, _ = fmt.Fprintln(out, gracefulCancelMessage)
		})
		if stop != nil {
			stop()
		}
	}
}

// executeRootCommand runs the root command under the supplied context while a
// background watcher translates cancellation into the graceful shutdown
// message. Tests can call this with a cancellable context directly instead of
// delivering real OS signals.
func executeRootCommand(rootCmd *cobra.Command, ctx context.Context, stop func()) error {
	cancelAnnounce := newGracefulCancelAnnouncer(rootCmd.ErrOrStderr(), stop)

	done := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			cancelAnnounce()
		case <-done:
		}
	}()

	err := rootCmd.ExecuteContext(ctx)
	close(done)
	<-watcherDone
	if ctx.Err() != nil {
		cancelAnnounce()
	}
	return err
}
