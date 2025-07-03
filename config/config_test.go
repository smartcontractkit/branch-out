package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionString(t *testing.T) {
	t.Parallel()

	versionString := VersionString()
	require.NotEmpty(t, versionString)
}

func TestConfig(t *testing.T) {
	t.Parallel()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)
}
