package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnitForEventType(t *testing.T) {
	assert.Equal(t, "in", unitForEventType("hail"))
	assert.Equal(t, "mph", unitForEventType("wind"))
	assert.Equal(t, "f_scale", unitForEventType("tornado"))
	assert.Empty(t, unitForEventType("unknown"))
}
