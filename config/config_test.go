package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spf13/viper"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

const (
	// This file does not exist. Set file to non-existent for tests where you want to make sure you don't load the real .env file.
	nonExistentFile = "non-existent-file.env"
)

func TestVersionString(t *testing.T) {
	t.Parallel()

	versionString := VersionString()
	require.NotEmpty(t, versionString)
}

func TestLoad(t *testing.T) {
	t.Parallel()

	l := testhelpers.Logger(t)
	cfg, err := Load(WithLogger(l))
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

func TestMustLoad(t *testing.T) {
	t.Parallel()

	l := testhelpers.Logger(t)
	require.NotPanics(t, func() {
		cfg := MustLoad(WithLogger(l))
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

	envContent := fmt.Sprintf(`BRANCH_OUT_LOG_LEVEL=%s
BRANCH_OUT_PORT=%d
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

func TestLoad_BadFile(t *testing.T) {
	t.Parallel()

	// Create a temporary .env file
	tempDir := t.TempDir()
	tempEnvFile := filepath.Join(tempDir, ".env")

	envContent := `bad-format`

	err := os.WriteFile(tempEnvFile, []byte(envContent), 0600)
	require.NoError(t, err)

	l := testhelpers.Logger(t)
	cfg, err := Load(WithConfigFile(tempEnvFile), WithLogger(l))
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
	t.Setenv("BRANCH_OUT_LOG_LEVEL", level)
	t.Setenv("BRANCH_OUT_PORT", fmt.Sprint(port))
	t.Setenv("GITHUB_TOKEN", token)
	t.Setenv("TRUNK_TOKEN", trunkToken)

	l := testhelpers.Logger(t)
	cfg, err := Load(WithConfigFile(nonExistentFile), WithLogger(l))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify the values were loaded from environment variables
	assert.Equal(t, level, cfg.LogLevel)
	assert.Equal(t, port, cfg.Port)
	assert.Equal(t, token, cfg.GitHub.Token)
	assert.Equal(t, trunkToken, cfg.Trunk.Token)
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	l := testhelpers.Logger(t)
	cfg, err := Load(WithLogger(l))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, DefaultLogLevel, cfg.LogLevel)
	assert.Equal(t, DefaultPort, cfg.Port)
}

func TestBindEnvsFromStruct(t *testing.T) {
	// Define test structs
	type NestedStruct struct {
		NestedField string `mapstructure:"NESTED_FIELD"`
		NoTag       string
	}

	type TestConfig struct {
		SimpleField   string       `mapstructure:"SIMPLE_FIELD"`
		NumberField   int          `mapstructure:"NUMBER_FIELD"`
		NoTagField    string       // should be ignored
		IgnoredField  string       `mapstructure:"-"`              // should be ignored
		EmptyTag      string       `mapstructure:""`               // should be ignored
		WhitespaceTag string       `mapstructure:"   "`            // should be ignored
		NestedSquash  NestedStruct `mapstructure:",squash"`        // should process nested fields
		RegularNested NestedStruct `mapstructure:"REGULAR_NESTED"` // should bind as single field
	}

	tests := []struct {
		name           string
		inputType      reflect.Type
		expectedEnv    map[string]string
		expectBound    []string
		expectNotBound []string
	}{
		{
			name:      "struct with various field types",
			inputType: reflect.TypeOf(TestConfig{}),
			expectedEnv: map[string]string{
				"SIMPLE_FIELD":   "test-value",
				"NUMBER_FIELD":   "42",
				"NESTED_FIELD":   "nested-value",
				"REGULAR_NESTED": "regular-nested-value",
			},
			expectBound: []string{
				"SIMPLE_FIELD",
				"NUMBER_FIELD",
				"NESTED_FIELD", // from squash
				"REGULAR_NESTED",
			},
			expectNotBound: []string{
				"NoTagField",    // no tag
				"IgnoredField",  // dash tag
				"EmptyTag",      // empty tag
				"WhitespaceTag", // whitespace tag
			},
		},
		{
			name:      "empty struct",
			inputType: reflect.TypeOf(struct{}{}),
			expectedEnv: map[string]string{
				"RANDOM_VAR": "should-not-bind",
			},
			expectBound:    []string{},
			expectNotBound: []string{"RANDOM_VAR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()

			// Set environment variables
			for envKey, envVal := range tt.expectedEnv {
				t.Setenv(envKey, envVal)
			}

			// Call the function under test
			err := bindEnvsFromStruct(v, tt.inputType)
			require.NoError(t, err)

			// Test that expected keys are bound and can read env vars
			for _, key := range tt.expectBound {
				envVal, exists := tt.expectedEnv[key]
				if exists {
					actualVal := v.GetString(key)
					assert.Equal(
						t,
						envVal,
						actualVal,
						"Expected '%s' to be bound to '%s'",
						key,
						envVal,
					)
				}
			}

			// Test that unexpected keys are not bound
			for _, key := range tt.expectNotBound {
				_, exists := tt.expectedEnv[key]
				if exists {
					actualVal := v.GetString(key)
					// If the key is not bound, viper should return empty string even if env var exists
					assert.Empty(
						t,
						actualVal,
						"Expected '%s' to NOT be bound",
						key,
					)
				}
			}
		})
	}
}

func TestBindEnvsFromStruct_ErrorHandling(t *testing.T) {
	t.Parallel()

	// Test that the function doesn't panic with various edge cases
	testCases := []struct {
		name string
		typ  reflect.Type
	}{
		{"nil type", nil},
		{"int type", reflect.TypeOf(42)},
		{"string type", reflect.TypeOf("hello")},
		{"slice type", reflect.TypeOf([]string{})},
		{"map type", reflect.TypeOf(map[string]string{})},
		{"function type", reflect.TypeOf(func() {})},
		{"interface type", reflect.TypeOf((*any)(nil)).Elem()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a new viper instance for this test
			v := viper.New()

			// This should not panic
			assert.NotPanics(t, func() {
				if tc.typ != nil {
					err := bindEnvsFromStruct(v, tc.typ)
					assert.Error(t, err)
				}
			})
		})
	}
}
