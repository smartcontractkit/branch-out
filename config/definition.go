package config

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	// ErrFieldNotFound is returned when a field is not found.
	ErrFieldNotFound = errors.New("field not found")

	// ErrMsgTypeMismatch is returned when the type of the default value, example value, or flag type does not match.
	ErrMsgTypeMismatch = "type mismatch for config field %s, flag type is '%s', default value type is '%s', example value type is '%s'"

	// ErrMsgUnsupportedType is returned when a type is not supported.
	ErrMsgUnsupportedType = "unsupported type %s for config flag %s, need to add support for this type in bindFlag"

	// ErrMsgDefaultNil is returned when a required field has no default value.
	ErrMsgDefaultNil = "default value is nil for required config field %s, need to set a default value"

	// ErrMsgFlagEmpty is returned when a field has no flag.
	ErrMsgFlagEmpty = "flag is empty for config field %s, need to set a flag"

	// ErrMsgEnvVarEmpty is returned when a field has no env var.
	ErrMsgEnvVarEmpty = "env var is empty for config field %s, need to set an env var"

	// ErrMsgTypeEmpty is returned when a field has no type.
	ErrMsgTypeEmpty = "type is empty for config field %s, need to set a type"
)

// Field represents a configuration field.
type Field struct {
	// EnvVar is the environment variable name. It is also the key in viper.
	EnvVar      string
	Description string
	Flag        string
	ShortFlag   string
	Type        reflect.Type
	Default     any
	Example     any
	Persistent  bool
	Required    bool
}

var (
	// Fields is a list of all configuration fields.
	Fields = append(coreFields, append(githubFields, append(trunkFields, jiraFields...)...)...)

	coreFields = []Field{
		{
			EnvVar:      "LOG_LEVEL",
			Description: "Log level for the application",
			Example:     "info",
			Flag:        "log-level",
			ShortFlag:   "l",
			Type:        reflect.TypeOf(""),
			Default:     "info",
			Persistent:  true,
		},
		{
			EnvVar:      "PORT",
			Description: "Port to listen on",
			Example:     8080,
			Flag:        "port",
			ShortFlag:   "p",
			Type:        reflect.TypeOf(0),
			Default:     8080,
			Persistent:  true,
		},
		{
			EnvVar:      "LOG_PATH",
			Description: "Path to a log file if you want to also log to a file",
			Default:     "",
			Example:     "/tmp/branch-out.log",
			Flag:        "log-path",
			Type:        reflect.TypeOf(""),
			Persistent:  true,
		},
	}

	githubFields = []Field{
		{
			EnvVar:      "GITHUB_TOKEN",
			Description: "GitHub personal access token, alternative to using a GitHub App",
			Example:     "ghp_xxxxxxxxxxxxxxxxxxxx",
			Flag:        "github-token",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "GITHUB_BASE_URL",
			Description: "GitHub API base URL",
			Example:     "https://api.github.com",
			Flag:        "github-base-url",
			Type:        reflect.TypeOf(""),
			Default:     "https://api.github.com",
		},
		{
			EnvVar:      "GITHUB_APP_ID",
			Description: "GitHub App ID, alternative to using a GitHub token",
			Example:     "123456",
			Flag:        "github-app-id",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "GITHUB_PRIVATE_KEY",
			Description: "GitHub App private key (PEM format)",
			Example:     "-----BEGIN RSA PRIVATE KEY-----\n<private-key-content>\n-----END RSA PRIVATE KEY-----",
			Flag:        "github-private-key",
			Type:        reflect.TypeOf(""),
			Default:     "",
		},
		{
			EnvVar:      "GITHUB_PRIVATE_KEY_FILE",
			Description: "Path to GitHub App private key file",
			Example:     "/path/to/private-key.pem",
			Flag:        "github-private-key-file",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "GITHUB_INSTALLATION_ID",
			Description: "GitHub App installation ID",
			Example:     "123456",
			Flag:        "github-installation-id",
			Type:        reflect.TypeOf(""),
		},
	}

	trunkFields = []Field{
		{
			EnvVar:      "TRUNK_TOKEN",
			Description: "API token for Trunk.io",
			Example:     "trunk_xxxxxxxxxxxxxxxxxxxx",
			Flag:        "trunk-token",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "TRUNK_WEBHOOK_SECRET",
			Description: "Webhook signing secret used to verify webhooks from Trunk.io",
			Example:     "trunk_webhook_secret",
			Flag:        "trunk-webhook-secret",
			Type:        reflect.TypeOf(""),
		},
	}

	jiraFields = []Field{
		{
			EnvVar:      "JIRA_BASE_DOMAIN",
			Description: "Jira base domain",
			Example:     "mycompany.atlassian.net",
			Flag:        "jira-base-domain",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "JIRA_PROJECT_KEY",
			Description: "Jira project key for tickets",
			Example:     "PROJ",
			Flag:        "jira-project-key",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "JIRA_OAUTH_CLIENT_ID",
			Description: "Jira OAuth client ID",
			Example:     "jira_oauth_client_id",
			Flag:        "jira-oauth-client-id",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "JIRA_OAUTH_CLIENT_SECRET",
			Description: "Jira OAuth client secret",
			Example:     "jira_oauth_client_secret",
			Flag:        "jira-oauth-client-secret",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "JIRA_OAUTH_ACCESS_TOKEN",
			Description: "Jira OAuth access token",
			Example:     "jira_oauth_access_token",
			Flag:        "jira-oauth-access-token",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "JIRA_OAUTH_REFRESH_TOKEN",
			Description: "Jira OAuth refresh token",
			Example:     "jira_oauth_refresh_token",
			Flag:        "jira-oauth-refresh-token",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "JIRA_USERNAME",
			Description: "Jira username for basic auth",
			Example:     "user@company.com",
			Flag:        "jira-username",
			Type:        reflect.TypeOf(""),
		},
		{
			EnvVar:      "JIRA_TOKEN",
			Description: "Jira API token for basic auth",
			Example:     "jira_api_token",
			Flag:        "jira-token",
			Type:        reflect.TypeOf(""),
		},
	}
)

func (f *Field) validate() error {
	if f.Flag == "" {
		return fmt.Errorf(ErrMsgFlagEmpty, f.Flag)
	}

	if f.EnvVar == "" {
		return fmt.Errorf(ErrMsgEnvVarEmpty, f.Flag)
	}

	if f.Type == nil {
		return fmt.Errorf(ErrMsgTypeEmpty, f.Flag)
	}

	if f.Default == nil && f.Required {
		return fmt.Errorf(ErrMsgDefaultNil, f.Flag)
	}

	// Check types match
	defaultType := reflect.TypeOf(f.Default)
	exampleType := reflect.TypeOf(f.Example)
	valueType := f.Type

	if f.Default != nil && defaultType != valueType {
		return fmt.Errorf(
			ErrMsgTypeMismatch,
			f.Flag,
			f.Type,
			defaultType,
			exampleType,
		)
	}

	if f.Example != nil && exampleType != valueType {
		return fmt.Errorf(
			ErrMsgTypeMismatch,
			f.Flag,
			f.Type,
			defaultType,
			exampleType,
		)
	}

	return nil
}

// GetField returns a configuration field by name.
func GetField(flag string) (Field, error) {
	for _, field := range Fields {
		if field.Flag == flag {
			return field, nil
		}
	}
	return Field{}, ErrFieldNotFound
}

// GetDefault returns the default value for a configuration field by name.
func GetDefault[T any](flag string) (T, error) {
	field, err := GetField(flag)
	if err != nil {
		var zero T
		return zero, err
	}

	value, ok := field.Default.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("type mismatch: expected %T, got %T", zero, field.Default)
	}

	return value, nil
}

// BindConfig binds the configuration to command flags and viper env vars.
func BindConfig(cmd *cobra.Command, v *viper.Viper) error {
	for _, field := range Fields {
		if err := bindField(cmd, v, field); err != nil {
			return err
		}
	}

	return nil
}

// MustBindConfig is BindConfig but panics if there is an error.
func MustBindConfig(cmd *cobra.Command, v *viper.Viper) {
	if err := BindConfig(cmd, v); err != nil {
		panic(err)
	}
}

// bindFlag binds a configuration field to a command flag.
func bindField(cmd *cobra.Command, v *viper.Viper, field Field) error {
	err := field.validate()
	if err != nil {
		return err
	}

	flag, err := buildFlag(cmd, field)
	if err != nil {
		return err
	}

	if v != nil && !v.IsSet(field.EnvVar) {
		if flag != nil {
			err = v.BindPFlag(field.EnvVar, flag)
			if err != nil {
				return err
			}
		}
		err = v.BindEnv(field.EnvVar, field.EnvVar)
		if err != nil {
			return err
		}
		if field.Default != nil {
			v.SetDefault(field.EnvVar, field.Default)
		}
	}

	return nil
}

// buildFlag builds a cobra flag from a field.
func buildFlag(cmd *cobra.Command, field Field) (*pflag.Flag, error) {
	// If nil command, don't bother setting the flag
	if cmd == nil {
		return nil, nil
	}

	if field.Flag == "" {
		return nil, fmt.Errorf("flag is empty")
	}

	flagSet := cmd.Flags()
	if field.Persistent {
		flagSet = cmd.PersistentFlags()
	}

	if flagSet.Lookup(field.Flag) != nil {
		return nil, nil // Flag already defined, don't set it again
	}

	switch field.Type {
	case reflect.TypeOf(""):
		var defaultValue string
		if field.Default != nil {
			defaultValue = field.Default.(string)
		}

		if field.ShortFlag != "" {
			flagSet.StringP(field.Flag, field.ShortFlag, defaultValue, field.Description)
		} else {
			flagSet.String(field.Flag, defaultValue, field.Description)
		}

	case reflect.TypeOf(0):
		var defaultValue int
		if field.Default != nil {
			defaultValue = field.Default.(int)
		}

		if field.ShortFlag != "" {
			flagSet.IntP(field.Flag, field.ShortFlag, defaultValue, field.Description)
		} else {
			flagSet.Int(field.Flag, defaultValue, field.Description)
		}

	case reflect.TypeOf(false):
		var defaultValue bool
		if field.Default != nil {
			defaultValue = field.Default.(bool)
		}

		if field.ShortFlag != "" {
			flagSet.BoolP(field.Flag, field.ShortFlag, defaultValue, field.Description)
		} else {
			flagSet.Bool(field.Flag, defaultValue, field.Description)
		}
	default:
		return nil, fmt.Errorf(ErrMsgUnsupportedType, field.Type, field.Flag)
	}

	return flagSet.Lookup(field.Flag), nil
}
