package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourcePressure_CheckFDUsageWarnsAtEightyPercent(t *testing.T) {
	check := CheckFDUsage(80, 100)

	assert.Equal(t, 80, check.FDUsed)
	assert.Equal(t, uint64(100), check.FDLimit)
	assert.Equal(t, 0.80, check.FDRatio)
	assert.Equal(t, ResourcePressureWarn, check.Severity)
}

func TestResourcePressure_CheckFDUsageOperatorAttentionAtNinetyPercent(t *testing.T) {
	check := CheckFDUsage(90, 100)

	assert.Equal(t, 90, check.FDUsed)
	assert.Equal(t, uint64(100), check.FDLimit)
	assert.Equal(t, 0.90, check.FDRatio)
	assert.Equal(t, ResourcePressureOperatorAttention, check.Severity)
}
