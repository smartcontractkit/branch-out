package quarantine_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/quarantine"
)

func TestFlaky(t *testing.T) {
	t.Run("skip flaky tests", func(t *testing.T) {
		t.Setenv("RUN_FLAKY_TESTS", "false")
		quarantine.Flaky(t, "TEST-123")
		didSkip := t.Skipped()

		require.True(t, didSkip, "quarantined test should be skipped when RUN_FLAKY_TESTS is false")
	})

	t.Run("run flaky tests", func(t *testing.T) {
		t.Setenv("RUN_FLAKY_TESTS", "true")
		quarantine.Flaky(t, "TEST-123")

		didSkip := t.Skipped()
		require.False(t, didSkip, "quarantined test should not be skipped when RUN_FLAKY_TESTS is true")
	})
}
