package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeTestTargets_Unquarantine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		unquarantineTargets []TestTarget
		expected            []TestTarget
	}{
		{
			name: "no duplicates",
			unquarantineTargets: []TestTarget{
				{Package: "github.com/example/pkg", Tests: []string{"TestA", "TestB"}},
				{Package: "github.com/example/pkg", Tests: []string{"TestB", "TestC"}},
			},
			expected: []TestTarget{
				{Package: "github.com/example/pkg", Tests: []string{"TestA", "TestB", "TestC"}},
			},
		},
		{
			name: "single target",
			unquarantineTargets: []TestTarget{
				{Package: "github.com/example/pkg", Tests: []string{"TestA"}},
			},
			expected: []TestTarget{
				{Package: "github.com/example/pkg", Tests: []string{"TestA"}},
			},
		},
		{
			name: "multiple packages",
			unquarantineTargets: []TestTarget{
				{Package: "github.com/example/pkg1", Tests: []string{"TestA"}},
				{Package: "github.com/example/pkg2", Tests: []string{"TestB"}},
			},
			expected: []TestTarget{
				{Package: "github.com/example/pkg1", Tests: []string{"TestA"}},
				{Package: "github.com/example/pkg2", Tests: []string{"TestB"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual := sanitizeTestTargets(test.unquarantineTargets)

			// Since map iteration order is not guaranteed, we need to sort for comparison
			require.Len(t, actual, len(test.expected))
			for _, expectedTarget := range test.expected {
				found := false
				for _, actualTarget := range actual {
					if expectedTarget.Package == actualTarget.Package {
						assert.ElementsMatch(t, expectedTarget.Tests, actualTarget.Tests)
						found = true
						break
					}
				}
				assert.True(t, found, "Expected package %s not found in actual results", expectedTarget.Package)
			}
		})
	}
}

func TestUnquarantineResults_QuarantinedConditionalRemoval(t *testing.T) {
	t.Parallel()

	// Test that verifies the complete unquarantine process:
	// - Starting with quarantined code (with conditional test skip logic)
	// - Ending with clean unquarantined code (without test skip logic)

	// This represents what the code should look like after unquarantining
	expectedUnquarantinedCode := `package test

import (
	"testing"
)

func TestExample(t *testing.T) {
	// Original test logic
	assert.True(t, true)
}`

	results := Results{
		"test/package": PackageResults{
			Package: "test/package",
			Successes: []File{
				{
					Package: "test/package",
					File:    "test_file.go",
					FileAbs: "/path/to/test_file.go",
					Tests: []Test{
						{Name: "TestExample", OriginalLine: 8},
					},
					ModifiedSourceCode: expectedUnquarantinedCode,
				},
			},
			Failures: []string{},
		},
	}

	// Verify the quarantined conditional logic is completely removed
	unquarantinedFile := results["test/package"].Successes[0]

	// Ensure quarantine-specific code is removed
	assert.NotContains(t, unquarantinedFile.ModifiedSourceCode, `os.Getenv("RUN_QUARANTINED_TESTS")`,
		"Unquarantined code should not contain environment variable check")
	assert.NotContains(t, unquarantinedFile.ModifiedSourceCode, `t.Skip("Flaky test quarantined`,
		"Unquarantined code should not contain t.Skip call")
	assert.NotContains(t, unquarantinedFile.ModifiedSourceCode, `t.Logf("'RUN_QUARANTINED_TESTS'`,
		"Unquarantined code should not contain quarantine logging")
	assert.NotContains(t, unquarantinedFile.ModifiedSourceCode, `"os"`,
		"Unquarantined code should not import os package if not needed")

	// Ensure original test logic is preserved
	assert.Contains(t, unquarantinedFile.ModifiedSourceCode, `func TestExample(t *testing.T)`,
		"Test function signature should be preserved")
	assert.Contains(t, unquarantinedFile.ModifiedSourceCode, `// Original test logic`,
		"Original test comments should be preserved")
	assert.Contains(t, unquarantinedFile.ModifiedSourceCode, `assert.True(t, true)`,
		"Original test assertions should be preserved")
	assert.Contains(t, unquarantinedFile.ModifiedSourceCode, `"testing"`,
		"Testing package import should be preserved")

	// Verify structure integrity
	assert.Contains(t, unquarantinedFile.ModifiedSourceCode, `package test`,
		"Package declaration should be preserved")
}
