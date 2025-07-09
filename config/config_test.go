package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spf13/viper"
)

func TestVersionString(t *testing.T) {
	t.Parallel()

	versionString := VersionString()
	require.NotEmpty(t, versionString)
}

func TestLoad(t *testing.T) {
	t.Parallel()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

func TestMustLoad(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		cfg := MustLoad()
		require.NotNil(t, cfg)
	})
}

func TestLoad_File(t *testing.T) {
	t.Parallel()

	// Create a temporary .env file
	tempDir := t.TempDir()
	tempEnvFile := filepath.Join(tempDir, ".env")

	var (
		level   = "test_level"
		port    = 9090
		baseURL = "https://test-base-url.com"
	)

	envContent := fmt.Sprintf(`LOG_LEVEL=%s
PORT=%d
GITHUB_BASE_URL=%s
`, level, port, baseURL)

	err := os.WriteFile(tempEnvFile, []byte(envContent), 0600)
	require.NoError(t, err)

	// Load config from the temporary file
	cfg, err := Load(WithConfigFile(tempEnvFile))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify the values were loaded correctly
	assert.Equal(t, level, cfg.LogLevel)
	assert.Equal(t, port, cfg.Port)
	assert.Equal(t, baseURL, cfg.GitHub.BaseURL)
}

func TestLoad_Viper(t *testing.T) {
	t.Parallel()

	v := viper.New()
	v.Set("LOG_LEVEL", "test_level")
	v.Set("PORT", 9090)
	v.Set("GITHUB_BASE_URL", "https://test-base-url.com")

	cfg, err := Load(WithViper(v))
	require.NoError(t, err, "error loading config from viper")
	require.NotNil(t, cfg, "config should not be nil")

	assert.Equal(t, "test_level", cfg.LogLevel)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "https://test-base-url.com", cfg.GitHub.BaseURL)
}

func TestLoad_BadFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	tempEnvFile := filepath.Join(tempDir, ".env")

	envContent := `bad-format`

	err := os.WriteFile(tempEnvFile, []byte(envContent), 0600)
	require.NoError(t, err)

	cfg, err := Load(WithConfigFile(tempEnvFile))
	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestLoad_EnvVars(t *testing.T) {
	const (
		level      = "env-level"
		port       = 42069
		token      = "env-token-456"
		trunkToken = "env-trunk-token-789"
	)

	t.Setenv("LOG_LEVEL", level)
	t.Setenv("PORT", fmt.Sprint(port))
	t.Setenv("GITHUB_TOKEN", token)
	t.Setenv("TRUNK_TOKEN", trunkToken)

	cfg, err := Load(WithConfigFile("non-existent-file.env"))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify the values were loaded from environment variables
	assert.Equal(t, level, cfg.LogLevel, "log level should be set from env var")
	assert.Equal(t, port, cfg.Port, "port should be set from env var")
	assert.Equal(t, token, cfg.GitHub.Token, "github token should be set from env var")
	assert.Equal(t, trunkToken, cfg.Trunk.Token, "trunk token should be set from env var")
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(WithConfigFile("non-existent-file.env"))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	logLevelField, err := GetField("log-level")
	require.NoError(t, err)

	assert.Equal(t, logLevelField.Default, cfg.LogLevel)

	portField, err := GetField("port")
	require.NoError(t, err)

	assert.Equal(t, portField.Default, cfg.Port)
}
