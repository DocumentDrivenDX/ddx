package agent

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIgnorableFetchOriginError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "legacy fetch origin error string",
			err:  errors.New("git fetch origin main: fatal: unable to access origin: exit status 128"),
			want: true,
		},
		{
			name: "legacy fetch origin error without branch",
			err:  errors.New("git fetch origin: fatal: unable to access origin: exit status 128"),
			want: true,
		},
		{
			name: "staged main worktree error",
			err:  errors.New("landing worktree has staged changes after waiting 2s:\nM\t.ddx/beads.jsonl"),
			want: false,
		},
		{
			name: "detached worktree error",
			err:  errors.New(`landing worktree is detached; expected branch "main"`),
			want: false,
		},
		{
			name: "branch mismatch error",
			err:  errors.New(`landing branch mismatch: on "feature", want "main"`),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsIgnorableFetchOriginError(tt.err))
		})
	}
}
