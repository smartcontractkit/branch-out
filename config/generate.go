package config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// generate.go is used to generate flags and docs for the config struct.
// Example struct tags for config: `mapstructure:"LOG_LEVEL" desc:"Log level for the application" example:"info" flag:"log-level" shortflag:"l" default:"info"`

// MustBindFlagsAndEnvs is BindFlagsAndEnvs but panics if there is an error
func MustBindFlagsAndEnvs(cmd *cobra.Command, v *viper.Viper) {
	// Process the main Config struct
	if err := BindFlagsAndEnvs(cmd, v); err != nil {
		panic(err)
	}
}

// BindFlagsAndEnvs processes the Config struct and sets up all flags and env var bindings needed
func BindFlagsAndEnvs(cmd *cobra.Command, v *viper.Viper) error {
	return processStruct(reflect.TypeOf(Config{}), cmd, v)
}

// BindFlags processes the Config struct and registers all flags for the Config struct
func BindFlags(cmd *cobra.Command) error {
	return processStruct(reflect.TypeOf(Config{}), cmd, nil)
}

// MustBindFlags is BindFlags but panics if there is an error
func MustBindFlags(cmd *cobra.Command) {
	if err := processStruct(reflect.TypeOf(Config{}), cmd, nil); err != nil {
		panic(err)
	}
}

// BindEnvs processes the Config struct and binds all env vars to the viper instance
func BindEnvs(v *viper.Viper) error {
	return processStruct(reflect.TypeOf(Config{}), nil, v)
}

// MustBindEnvs processes the Config struct and binds all env vars to the viper instance
func MustBindEnvs(v *viper.Viper) {
	if err := processStruct(reflect.TypeOf(Config{}), nil, v); err != nil {
		panic(err)
	}
}

// processConfig recursively extracts pflags and env var values from the Config struct fields using reflection
func processStruct(t reflect.Type, cmd *cobra.Command, v *viper.Viper) error {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct, got %s", t.Kind())
	}

	flagSet := pflag.NewFlagSet(t.Name(), pflag.ContinueOnError)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Check if this field has squash tag (embedded struct)
		mapStructTag := field.Tag.Get("mapstructure")
		if strings.Contains(mapStructTag, ",squash") {
			// Recursively process embedded struct
			if err := processStruct(field.Type, cmd, v); err != nil {
				return err
			}
			continue
		}

		if err := processField(flagSet, v, field); err != nil {
			return err
		}
	}

	if cmd != nil {
		cmd.PersistentFlags().AddFlagSet(flagSet)
	}

	return nil
}

func processField(flagSet *pflag.FlagSet, v *viper.Viper, field reflect.StructField) error {
	// Extract flag information from tags
	flagName := field.Tag.Get("flag")
	desc := field.Tag.Get("desc")
	envVar := field.Tag.Get("mapstructure")

	if flagName == "" {
		return fmt.Errorf("flag name is empty")
	}

	if desc == "" {
		return fmt.Errorf("description is empty")
	}

	if flagSet.Lookup(flagName) != nil {
		return fmt.Errorf("flag %s already exists", flagName)
	}

	fullDesc := fmt.Sprintf("%s (env: %s)", desc, envVar)
	// Create the appropriate flag based on field type
	switch field.Type.Kind() {
	case reflect.String:
		defaultValue := field.Tag.Get("default")
		flagSet.String(flagName, defaultValue, fullDesc)
	case reflect.Int:
		defaultValue := 0
		if field.Tag.Get("default") != "" {
			var err error
			defaultValue, err = strconv.Atoi(field.Tag.Get("default"))
			if err != nil {
				return fmt.Errorf("invalid default value for int field: %w", err)
			}

		}
		flagSet.Int(flagName, defaultValue, fullDesc)

	case reflect.Bool:
		defaultValue := false
		if field.Tag.Get("default") != "" {
			var err error
			defaultValue, err = strconv.ParseBool(field.Tag.Get("default"))
			if err != nil {
				return fmt.Errorf("invalid default value for bool field: %w", err)
			}
		}
		flagSet.Bool(flagName, defaultValue, fullDesc)

	default:
		return fmt.Errorf("unsupported field type: %s", field.Type.Kind())
	}

	if v != nil {
		if err := v.BindPFlag(envVar, flagSet.Lookup(flagName)); err != nil {
			return fmt.Errorf("failed to bind flag %s to env var %s: %w", flagName, envVar, err)
		}
		if err := v.BindEnv(envVar); err != nil {
			return fmt.Errorf("failed to bind env var %s: %w", envVar, err)
		}
	}
	return nil
}
