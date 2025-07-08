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

// Default config values
const (
	// DefaultPort is the default port for the server to listen on.
	DefaultPort = 8080
	// DefaultLogLevel is the default log level for the server.
	DefaultLogLevel = "info"
	// DefaultGitHubBaseURL is the default base URL for the GitHub API.
	DefaultGitHubBaseURL = "https://api.github.com"
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
	LogLevel string `mapstructure:"LOG_LEVEL" desc:"Log level for the application"    example:"info"                flag:"log-level" shortflag:"l" default:"info"`
	LogPath  string `mapstructure:"LOG_PATH"  desc:"Path to log file (optional)"      example:"/tmp/branch-out.log" flag:"log-path"`
	Port     int    `mapstructure:"PORT"      desc:"Port for the server to listen on" example:"8080"                flag:"port"      shortflag:"p" default:"8080"`

	// Secrets
	GitHub GitHub `mapstructure:",squash"`
	Trunk  Trunk  `mapstructure:",squash"`
	Jira   Jira   `mapstructure:",squash"`
}

// GitHub configures authentication to the GitHub API.
type GitHub struct {
	BaseURL string `mapstructure:"GITHUB_BASE_URL"         env:"GITHUB_BASE_URL"         desc:"GitHub API base URL"                                        example:"https://api.github.com"   flag:"github-base-url"         default:"https://api.github.com"`
	// GitHub App configuration
	AppID          string `mapstructure:"GITHUB_APP_ID"           env:"GITHUB_APP_ID"           desc:"GitHub App ID (alternative to token)"                       example:"123456"                   flag:"github-app-id"`
	PrivateKey     string `mapstructure:"GITHUB_PRIVATE_KEY"      env:"GITHUB_PRIVATE_KEY"      desc:"GitHub App private key (PEM format)"                                                           flag:"github-private-key"`
	PrivateKeyFile string `mapstructure:"GITHUB_PRIVATE_KEY_FILE" env:"GITHUB_PRIVATE_KEY_FILE" desc:"Path to GitHub App private key file"                        example:"/path/to/private-key.pem" flag:"github-private-key-file"`
	InstallationID string `mapstructure:"GITHUB_INSTALLATION_ID"  env:"GITHUB_INSTALLATION_ID"  desc:"GitHub App installation ID"                                 example:"87654321"                 flag:"github-installation-id"`
	// Or use a simple GitHub token
	Token string `mapstructure:"GITHUB_TOKEN"            env:"GITHUB_TOKEN"            desc:"GitHub personal access token instead of using a GitHub App" example:"ghp_xxxxxxxxxxxxxxxxxxxx" flag:"github-token"`
}

// Trunk configures authentication to the Trunk API.
type Trunk struct {
	Token string `mapstructure:"TRUNK_TOKEN" env:"TRUNK_TOKEN" desc:"Trunk API token" example:"trunk_xxxxxxxxxxxxxxxxxxxx" flag:"trunk-token"`
}

// Jira configures authentication to the Jira API.
type Jira struct {
	BaseDomain        string `mapstructure:"JIRA_BASE_DOMAIN"         env:"JIRA_BASE_DOMAIN"         desc:"Jira base domain"              example:"mycompany.atlassian.net"  flag:"jira-base-domain"`
	ProjectKey        string `mapstructure:"JIRA_PROJECT_KEY"         env:"JIRA_PROJECT_KEY"         desc:"Jira project key for tickets"  example:"PROJ"                     flag:"jira-project-key"`
	OAuthClientID     string `mapstructure:"JIRA_OAUTH_CLIENT_ID"     env:"JIRA_OAUTH_CLIENT_ID"     desc:"Jira OAuth client ID"          example:"jira_oauth_client_id"     flag:"jira-oauth-client-id"`
	OAuthClientSecret string `mapstructure:"JIRA_OAUTH_CLIENT_SECRET" env:"JIRA_OAUTH_CLIENT_SECRET" desc:"Jira OAuth client secret"      example:"jira_oauth_client_secret" flag:"jira-oauth-client-secret"`
	OAuthAccessToken  string `mapstructure:"JIRA_OAUTH_ACCESS_TOKEN"  env:"JIRA_OAUTH_ACCESS_TOKEN"  desc:"Jira OAuth access token"       example:"jira_oauth_access_token"  flag:"jira-oauth-access-token"`
	OAuthRefreshToken string `mapstructure:"JIRA_OAUTH_REFRESH_TOKEN" env:"JIRA_OAUTH_REFRESH_TOKEN" desc:"Jira OAuth refresh token"      example:"jira_oauth_refresh_token" flag:"jira-oauth-refresh-token"`
	Username          string `mapstructure:"JIRA_USERNAME"            env:"JIRA_USERNAME"            desc:"Jira username for basic auth"  example:"user@company.com"         flag:"jira-username"`
	Token             string `mapstructure:"JIRA_TOKEN"               env:"JIRA_TOKEN"               desc:"Jira API token for basic auth" example:"jira_api_token"           flag:"jira-token"`
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
		command:    &cobra.Command{},
	}
	for _, opt := range options {
		opt(opts)
	}

	v := opts.viper
	if v == nil {
		v = viper.New()
	}
	err := BindFlagsAndEnvs(opts.command, v)
	if err != nil {
		return nil, err
	}
	v.AutomaticEnv()

	if opts.configFile != "" {
		v.SetConfigFile(opts.configFile)
	}

	err = v.ReadInConfig()
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
