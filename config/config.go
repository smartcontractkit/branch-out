// Package config provides the configuration for the application.
package config

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/spf13/viper"
)

// These variables are set at build time and describe the version and build of the application
var (
	Version   string
	Commit    string
	BuildTime = time.Now().Format("2006-01-02T15:04:05.000")
	BuiltBy   = "local"
	BuiltWith = runtime.Version()
)

// VersionString gives a full string of the version of the application.
func VersionString() string {
	return fmt.Sprintf(
		"%s on commit %s, built at %s with %s by %s",
		Version,
		Commit,
		BuildTime,
		BuiltWith,
		BuiltBy,
	)
}

// Config is the application configuration, set by flags, then by environment variables.
type Config struct {
	LogLevel string `mapstructure:"BRANCH_OUT_LOG_LEVEL"`
	LogPath  string `mapstructure:"BRANCH_OUT_LOG_PATH"`
	Port     int    `mapstructure:"BRANCH_OUT_PORT"`

	// Secrets
	GitHub GitHub
	Trunk  Trunk
	Jira   Jira
}

// GitHub configures authentication to the GitHub API.
type GitHub struct {
	BaseURL string `mapstructure:"GITHUB_BASE_URL"`
	// GitHub App configuration
	AppID          string `mapstructure:"GITHUB_APP_ID"`
	PrivateKey     string `mapstructure:"GITHUB_PRIVATE_KEY"`
	PrivateKeyFile string `mapstructure:"GITHUB_PRIVATE_KEY_FILE"`
	InstallationID string `mapstructure:"GITHUB_INSTALLATION_ID"`
	// Or use a simple GitHub token
	Token string `mapstructure:"GITHUB_TOKEN"`
}

// Trunk configures authentication to the Trunk API.
type Trunk struct {
	Token string `mapstructure:"TRUNK_TOKEN"`
}

// Jira configures authentication to the Jira API.
type Jira struct {
	BaseDomain        string `mapstructure:"JIRA_BASE_DOMAIN"`
	ProjectKey        string `mapstructure:"JIRA_PROJECT_KEY"`
	OAuthClientID     string `mapstructure:"JIRA_OAUTH_CLIENT_ID"`
	OAuthClientSecret string `mapstructure:"JIRA_OAUTH_CLIENT_SECRET"`
	OAuthAccessToken  string `mapstructure:"JIRA_OAUTH_ACCESS_TOKEN"`
	OAuthRefreshToken string `mapstructure:"JIRA_OAUTH_REFRESH_TOKEN"`
	Username          string `mapstructure:"JIRA_USERNAME"`
	Token             string `mapstructure:"JIRA_TOKEN"`
}

// Load loads config from environment variables and flags.
func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	// Ignore errors if the config file doesn't exist or is not found. Just use env vars and flags.
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, viper.ConfigFileNotFoundError{}) {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func init() {
	// Version setup
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		if Version == "" {
			Version = buildInfo.Main.Version
		}
		if Commit == "" {
			Commit = buildInfo.Main.Sum
		}
		BuiltWith = buildInfo.GoVersion
	}
	if Version == "" {
		Version = "dev"
	}
	if Commit == "" {
		Commit = "dev"
	}

	// Config setup
	viper.SetDefault("GITHUB_BASE_URL", "https://api.github.com")
}
