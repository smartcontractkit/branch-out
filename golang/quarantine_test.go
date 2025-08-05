package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeTestTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		quarantineTargets []TestTarget
		expected          []TestTarget
	}{
		{
			name: "no duplicates",
			quarantineTargets: []TestTarget{
				{Package: "github.com/example/pkg", Tests: []string{"TestA", "TestB"}},
				{Package: "github.com/example/pkg", Tests: []string{"TestB", "TestC"}},
			},
			expected: []TestTarget{
				{Package: "github.com/example/pkg", Tests: []string{"TestA", "TestB", "TestC"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual := sanitizeTestTargets(test.quarantineTargets)
			assert.Equal(t, test.expected, actual)
		})
	}
}
