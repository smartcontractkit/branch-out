// Package config provides the configuration for the application.
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// DefaultPort is the default port for the server to listen on.
const (
	// DefaultPort is the default port for the server to listen on.
	DefaultPort = 8181
	// DefaultLogLevel is the default log level for the server.
	DefaultLogLevel = "info"
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
	GitHub GitHub `mapstructure:",squash"`
	Trunk  Trunk  `mapstructure:",squash"`
	Jira   Jira   `mapstructure:",squash"`
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

// Option is a function that can be used to configure loading the config.
type Option func(*configOptions)

type configOptions struct {
	configFile string
	logger     zerolog.Logger
	viper      *viper.Viper
}

// WithConfigFile sets the exact config file to load.
func WithConfigFile(configFile string) Option {
	return func(cfg *configOptions) {
		cfg.configFile = configFile
	}
}

// WithLogger sets the logger to use.
func WithLogger(logger zerolog.Logger) Option {
	return func(cfg *configOptions) {
		cfg.logger = logger
	}
}

// WithViper sets a custom viper instance to use. Useful for testing.
func WithViper(v *viper.Viper) Option {
	return func(cfg *configOptions) {
		cfg.viper = v
	}
}

// Load loads config from environment variables and flags.
func Load(options ...Option) (*Config, error) {
	opts := &configOptions{
		configFile: ".env",
		logger:     zerolog.Nop(),
		viper:      nil, // Will create new instance if not provided
	}
	for _, opt := range options {
		opt(opts)
	}
	l := opts.logger.With().Str("config_file", opts.configFile).Logger()

	// Use provided viper instance or create a new one
	v := opts.viper
	if v == nil {
		v = viper.New()
		// Set up defaults and env binding for new instance
		setupViperDefaults(v)
	}

	if opts.configFile != "" {
		v.SetConfigFile(opts.configFile)
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok || errors.Is(err, os.ErrNotExist) {
			// Config file not found; ignore error and use env vars and flags
			l.Debug().Msg("Config file not found")
		} else {
			l.Error().Err(err).Msg("Error reading config file")
			// Config file was found but another error was produced
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	l.Debug().Str("log_level", cfg.LogLevel).Int("port", cfg.Port).Msg("Loaded config")

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

	// Set up defaults for global viper instance (for backward compatibility)
	setupViperDefaults(viper.GetViper())
}

// setupViperDefaults configures viper with sensible defaults for all configuration fields
func setupViperDefaults(v *viper.Viper) {
	// Set only the essential defaults
	v.SetDefault("BRANCH_OUT_LOG_LEVEL", DefaultLogLevel)
	v.SetDefault("BRANCH_OUT_PORT", DefaultPort)
	v.SetDefault("GITHUB_BASE_URL", "https://api.github.com")

	// Automatically bind all environment variables based on struct tags
	if err := bindEnvsFromStruct(v, reflect.TypeOf(Config{})); err != nil {
		panic(err)
	}

	// Configure viper to automatically read environment variables
	v.AutomaticEnv()
}

// bindEnvsFromStruct binds environment variables to viper based on struct tags.
// Avoids having to manually viper.BindEnv for each field.
func bindEnvsFromStruct(v *viper.Viper, t reflect.Type) error {
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("type %s is not a struct", t.Name())
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")
		// Skip fields without a mapstructure tag
		if tag == "" {
			continue
		}
		if strings.Contains(tag, ",squash") {
			// Handle embedded structs with squash
			if err := bindEnvsFromStruct(v, field.Type); err != nil {
				return err
			}
			continue
		}
		if tag == "-" {
			// Skip ignored fields
			continue
		}
		// Bind the environment variable
		if err := v.BindEnv(tag); err != nil {
			return fmt.Errorf("failed to bind env %s: %w", tag, err)
		}
	}
	return nil
}
