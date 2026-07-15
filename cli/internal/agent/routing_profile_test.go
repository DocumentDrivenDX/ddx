package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeRoutingProfile_EmptyStaysEmpty(t *testing.T) {
	assert.Empty(t, NormalizeRoutingProfile(""))
	assert.Equal(t, "  ", NormalizeRoutingProfile("  "))
	assert.Equal(t, " default ", NormalizeRoutingProfile(" default "))
	assert.Equal(t, " smart ", NormalizeRoutingProfile(" smart "))
}
