// Package config provides the configuration for the application.
package config

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/spf13/cobra"
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
	LogLevel string `mapstructure:"LOG_LEVEL"`
	LogPath  string `mapstructure:"LOG_PATH"`
	Port     int    `mapstructure:"PORT"`

	// Secrets
	GitHub GitHub `mapstructure:",squash"`
	Trunk  Trunk  `mapstructure:",squash"`
	Jira   Jira   `mapstructure:",squash"`
}

// GitHub configures authentication to the GitHub API.
type GitHub struct {
	Token   string `mapstructure:"GITHUB_TOKEN"`
	BaseURL string `mapstructure:"GITHUB_BASE_URL"`
	// GitHub App configuration
	AppID          string `mapstructure:"GITHUB_APP_ID"`
	PrivateKey     string `mapstructure:"GITHUB_PRIVATE_KEY"`
	PrivateKeyFile string `mapstructure:"GITHUB_PRIVATE_KEY_FILE"`
	InstallationID string `mapstructure:"GITHUB_INSTALLATION_ID"`
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

// Option is a function that can be used to configure loading the config.
type Option func(*configOptions)

type configOptions struct {
	configFile string
	viper      *viper.Viper
	command    *cobra.Command
}

// WithConfigFile sets the exact config file to load.
func WithConfigFile(configFile string) Option {
	return func(cfg *configOptions) {
		cfg.configFile = configFile
	}
}

// WithViper sets the viper instance to use. A new viper instance is created if not provided.
func WithViper(v *viper.Viper) Option {
	return func(cfg *configOptions) {
		cfg.viper = v
	}
}

// WithCommand sets the command to use for binding flags to config values.
func WithCommand(cmd *cobra.Command) Option {
	return func(cfg *configOptions) {
		cfg.command = cmd
	}
}

// Load loads config from environment variables and flags.
func Load(options ...Option) (*Config, error) {
	opts := &configOptions{
		configFile: ".env",
		viper:      viper.New(),
		command:    nil,
	}
	for _, opt := range options {
		opt(opts)
	}

	v := opts.viper
	if err := BindConfig(opts.command, v); err != nil {
		return nil, err
	}

	if opts.configFile != "" {
		v.SetConfigFile(opts.configFile)
	}

	err := v.ReadInConfig()
	if err != nil {
		// Ignore config file not found error
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	var cfg Config
	err = v.Unmarshal(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// MustLoad is Load but panics if there is an error.
func MustLoad(options ...Option) *Config {
	cfg, err := Load(options...)
	if err != nil {
		panic(err)
	}
	return cfg
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
}
