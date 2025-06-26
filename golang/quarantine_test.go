package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeQuarantineTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		quarantineTargets []QuarantineTarget
		expected          []QuarantineTarget
	}{
		{
			name: "no duplicates",
			quarantineTargets: []QuarantineTarget{
				{Package: "github.com/example/pkg", Tests: []string{"TestA", "TestB"}},
				{Package: "github.com/example/pkg", Tests: []string{"TestB", "TestC"}},
			},
			expected: []QuarantineTarget{
				{Package: "github.com/example/pkg", Tests: []string{"TestA", "TestB", "TestC"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual := sanitizeQuarantineTargets(test.quarantineTargets)
			assert.Equal(t, test.expected, actual)
		})
	}
}
