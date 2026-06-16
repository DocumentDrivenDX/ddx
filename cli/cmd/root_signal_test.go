package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootSignal_FirstInterruptCancelsContextAndPrintsGracefulMessage(t *testing.T) {
	root := &cobra.Command{
		Use:           "ddx",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	started := make(chan struct{})
	root.AddCommand(&cobra.Command{
		Use:           "block",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			close(started)
			<-cmd.Context().Done()
			return cmd.Context().Err()
		},
	})
	root.SetArgs([]string{"block"})

	var stderr bytes.Buffer
	root.SetErr(&stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stopCalled := make(chan struct{}, 1)
	stop := func() {
		select {
		case stopCalled <- struct{}{}:
		default:
		}
	}
	announce := newGracefulCancelAnnouncer(&stderr, stop)

	done := make(chan error, 1)
	go func() {
		done <- root.ExecuteContext(ctx)
	}()

	<-started
	cancel()
	announce()

	err := <-done
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, strings.Count(stderr.String(), gracefulCancelMessage))
	assert.Equal(t, gracefulCancelMessage+"\n", stderr.String())
	select {
	case <-stopCalled:
	default:
		t.Fatal("stop must be called after the first cancel")
	}
}

func TestRootSignal_MessagePrintedOnce(t *testing.T) {
	var stderr bytes.Buffer
	announce := newGracefulCancelAnnouncer(&stderr, nil)

	announce()
	announce()

	assert.Equal(t, gracefulCancelMessage+"\n", stderr.String())
}

func TestExecuteRootCommand_NormalCompletionDoesNotPrintCancelMessage(t *testing.T) {
	root := &cobra.Command{
		Use:           "ddx",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	var stderr bytes.Buffer
	root.SetErr(&stderr)

	err := executeRootCommand(root, context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}
